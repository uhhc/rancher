package eks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"encoding/base64"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client/metadata"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ec2"
	heptio "github.com/heptio/authenticator/pkg/token"
	"github.com/rancher/kontainer-engine/drivers/util"
	"github.com/rancher/kontainer-engine/types"
	"github.com/sirupsen/logrus"
	"github.com/smartystreets/go-aws-auth"
	"gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Driver struct {
	types.UnimplementedClusterSizeAccess
	types.UnimplementedVersionAccess

	driverCapabilities types.Capabilities

	request.Retryer
	metadata.ClientInfo

	Config   aws.Config
	Handlers request.Handlers
}

type state struct {
	ClusterName  string
	DisplayName  string
	ClientID     string
	ClientSecret string

	ClusterInfo types.ClusterInfo
}

type eksCluster struct {
	Cluster clusterObj `json:"cluster"`
}

type clusterObj struct {
	MasterEndpoint       *string              `json:"masterEndpoint"`
	ClusterName          *string              `json:"clusterName"`
	Status               *string              `json:"status"`
	CreatedAt            *int                 `json:"createdAt"`
	DesiredMasterVerion  *string              `json:"desiredMasterVersion"`
	VPCID                *string              `json:"vpcId"`
	CurrentMasterVersion *string              `json:"currentMasterVersion"`
	RoleARN              *string              `json:"roleArn"`
	CertificateAuthority certificateAuthority `json:"certificateAuthority"`
	SecurityGroups       []string             `json:"securityGroups"`
	Subnets              []string             `json:"subnets"`
}

type certificateAuthority struct {
	Data *string `json:"data"`
}

func NewDriver() types.Driver {
	driver := &Driver{
		driverCapabilities: types.Capabilities{
			Capabilities: make(map[int64]bool),
		},
	}

	return driver
}

func (d *Driver) GetDriverCreateOptions(ctx context.Context) (*types.DriverFlags, error) {
	driverFlag := types.DriverFlags{
		Options: make(map[string]*types.Flag),
	}
	driverFlag.Options["display-name"] = &types.Flag{
		Type:  types.StringType,
		Usage: "The displayed name of the cluster in the Rancher UI",
	}
	driverFlag.Options["client-id"] = &types.Flag{
		Type:  types.StringType,
		Usage: "The AWS Client ID to use",
	}
	driverFlag.Options["client-secret"] = &types.Flag{
		Type:  types.StringType,
		Usage: "The AWS Client Secret associated with the Client ID",
	}

	return &driverFlag, nil
}

func (d *Driver) GetDriverUpdateOptions(ctx context.Context) (*types.DriverFlags, error) {
	driverFlag := types.DriverFlags{
		Options: make(map[string]*types.Flag),
	}

	return &driverFlag, nil
}

func getStateFromOptions(driverOptions *types.DriverOptions) (state, error) {
	logrus.Infof("%v", driverOptions)
	state := state{}
	state.ClusterName = getValueFromDriverOptions(driverOptions, types.StringType, "name").(string)
	state.DisplayName = getValueFromDriverOptions(driverOptions, types.StringType, "display-name", "displayName").(string)
	state.ClientID = getValueFromDriverOptions(driverOptions, types.StringType, "client-id", "accessKey").(string)
	state.ClientSecret = getValueFromDriverOptions(driverOptions, types.StringType, "client-secret", "secretKey").(string)

	return state, state.validate()
}

func getValueFromDriverOptions(driverOptions *types.DriverOptions, optionType string, keys ...string) interface{} {
	switch optionType {
	case types.IntType:
		for _, key := range keys {
			if value, ok := driverOptions.IntOptions[key]; ok {
				return value
			}
		}
		return int64(0)
	case types.StringType:
		for _, key := range keys {
			if value, ok := driverOptions.StringOptions[key]; ok {
				return value
			}
		}
		return ""
	case types.BoolType:
		for _, key := range keys {
			if value, ok := driverOptions.BoolOptions[key]; ok {
				return value
			}
		}
		return false
	case types.StringSliceType:
		for _, key := range keys {
			if value, ok := driverOptions.StringSliceOptions[key]; ok {
				return value
			}
		}
		return &types.StringSlice{}
	}
	return nil
}

func (state *state) validate() error {
	if state.ClusterName == "" {
		return fmt.Errorf("cluster name is required")
	}

	if state.ClientID == "" {
		return fmt.Errorf("client id is required")
	}

	if state.ClientSecret == "" {
		return fmt.Errorf("client secret is required")
	}

	return nil
}

func alreadyExistsInCloudFormationError(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		case cloudformation.ErrCodeAlreadyExistsException:
			return true
		}
	}

	return false
}

func (d *Driver) createStack(svc *cloudformation.CloudFormation, name string, displayName string,
	templateURL string, parameters []*cloudformation.Parameter) (*cloudformation.DescribeStacksOutput, error) {
	_, err := svc.CreateStack(&cloudformation.CreateStackInput{
		StackName:   aws.String(name),
		TemplateURL: aws.String(templateURL),
		Capabilities: aws.StringSlice([]string{
			cloudformation.CapabilityCapabilityIam,
		}),
		Parameters: parameters,
		Tags: []*cloudformation.Tag{
			{Key: aws.String("displayName"), Value: aws.String(displayName)},
		},
	})
	if err != nil && !alreadyExistsInCloudFormationError(err) {
		return nil, fmt.Errorf("error creating master: %v", err)
	}

	var stack *cloudformation.DescribeStacksOutput
	status := "CREATE_IN_PROGRESS"

	for status == "CREATE_IN_PROGRESS" {
		time.Sleep(time.Second * 5)
		stack, err = svc.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: aws.String(name),
		})
		if err != nil {
			return nil, fmt.Errorf("error polling stack info: %v", err)
		}

		status = *stack.Stacks[0].StackStatus
	}

	if len(stack.Stacks) == 0 {
		return nil, fmt.Errorf("stack did not have output: %v", err)
	}

	return stack, nil
}

func (d *Driver) awsHTTPRequest(state state, url string, method string, data []byte) ([]byte, error) {
	req, err := http.NewRequest(method, url,
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("error creating http request: %v", err)
	}

	client := http.DefaultClient

	req.Header.Set("Content-Type", "application/json")

	awsauth.Sign4(req, awsauth.Credentials{
		AccessKeyID:     state.ClientID,
		SecretAccessKey: state.ClientSecret,
	})

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error creating cluster: %v", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading body: %v", err)
	}

	return body, nil
}

func (d *Driver) Create(ctx context.Context, options *types.DriverOptions) (*types.ClusterInfo, error) {
	logrus.Infof("Starting create")

	state, err := getStateFromOptions(options)
	if err != nil {
		return nil, fmt.Errorf("error parsing state: %v", err)
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"),
		Credentials: credentials.NewStaticCredentials(
			state.ClientID,
			state.ClientSecret,
			"",
		),
	})
	if err != nil {
		return nil, fmt.Errorf("error getting new aws session: %v", err)
	}

	svc := cloudformation.New(sess)

	logrus.Infof("Bringing up vpc")

	displayName := state.DisplayName
	if displayName == "" {
		displayName = state.ClusterName
	}

	stack, err := d.createStack(svc, getVPCStackName(state), displayName,
		"https://amazon-eks.s3-us-west-2.amazonaws.com/2018-04-04/amazon-eks-vpc-sample.yaml",
		[]*cloudformation.Parameter{
			{ParameterKey: aws.String("ClusterName"), ParameterValue: aws.String(state.ClusterName)},
		})
	if err != nil {
		return nil, fmt.Errorf("error creating stack: %v", err)
	}

	securityGroups := getParameterValueFromOutput("SecurityGroups", stack.Stacks[0].Outputs)
	subnetIds := getParameterValueFromOutput("SubnetIds", stack.Stacks[0].Outputs)

	if securityGroups == "" || subnetIds == "" {
		return nil, fmt.Errorf("no security groups or subnet ids were returned")
	}

	resources, err := svc.DescribeStackResources(&cloudformation.DescribeStackResourcesInput{
		StackName: aws.String(state.ClusterName + "-eks-vpc"),
	})
	if err != nil {
		return nil, fmt.Errorf("error getting stack resoures")
	}

	var vpcid string
	for _, resource := range resources.StackResources {
		if *resource.LogicalResourceId == "VPC" {
			vpcid = *resource.PhysicalResourceId
		}
	}

	logrus.Infof("Creating service role")

	stack, err = d.createStack(svc, getServiceRoleName(state), displayName,
		"https://amazon-eks.s3-us-west-2.amazonaws.com/2018-04-04/amazon-eks-service-role.yaml", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating stack: %v", err)
	}

	roleARN := getParameterValueFromOutput("RoleArn", stack.Stacks[0].Outputs)
	if roleARN == "" {
		return nil, fmt.Errorf("no RoleARN was returned")
	}

	data, err := json.Marshal(&clusterObj{
		ClusterName:    aws.String(state.ClusterName),
		RoleARN:        aws.String(roleARN),
		SecurityGroups: strings.Split(securityGroups, " "),
		Subnets:        strings.Split(subnetIds, " "),
	})
	if err != nil {
		return nil, fmt.Errorf("error marshalling eks cluster: %v", err)
	}

	logrus.Infof("Creating EKS cluster")

	_, err = d.awsHTTPRequest(state, "https://eks.us-west-2.amazonaws.com/clusters", "POST", data)
	if err != nil && !isClusterConflict(err) {
		return nil, fmt.Errorf("error posting cluster: %v", err)
	}

	cluster, err := d.waitForClusterReady(state)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Cluster provisioned successfully")

	capem, err := base64.StdEncoding.DecodeString(*cluster.Cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, fmt.Errorf("error parsing CA data: %v", err)
	}

	ec2svc := ec2.New(sess)
	keyPairName := getEC2KeyPairName(state)
	_, err = ec2svc.CreateKeyPair(&ec2.CreateKeyPairInput{
		KeyName: aws.String(keyPairName),
	})
	if err != nil && !isDuplicateKeyError(err) {
		return nil, fmt.Errorf("error creating key pair %v", err)
	}

	logrus.Infof("Creating worker nodes")

	stack, err = d.createStack(svc, getWorkNodeName(state), displayName,
		"https://amazon-eks.s3-us-west-2.amazonaws.com/2018-04-04/amazon-eks-nodegroup.yaml",
		[]*cloudformation.Parameter{
			{ParameterKey: aws.String("ClusterName"), ParameterValue: aws.String(state.ClusterName)},
			{ParameterKey: aws.String("ClusterControlPlaneSecurityGroup"),
				ParameterValue: aws.String(securityGroups)},
			{ParameterKey: aws.String("NodeGroupName"),
				ParameterValue: aws.String(state.ClusterName + "-node-group")}, //
			{ParameterKey: aws.String("NodeAutoScalingGroupMinSize"), ParameterValue: aws.String("1")}, // TODO let the user specify this
			{ParameterKey: aws.String("NodeAutoScalingGroupMaxSize"), ParameterValue: aws.String("3")}, // TODO let the user specify this
			{ParameterKey: aws.String("NodeInstanceType"), ParameterValue: aws.String("m4.large")},     // TODO let the user specify this
			{ParameterKey: aws.String("NodeImageId"), ParameterValue: aws.String("ami-e09b0098")},
			{ParameterKey: aws.String("KeyName"), ParameterValue: aws.String(keyPairName)}, // TODO let the user specify this
			{ParameterKey: aws.String("VpcId"), ParameterValue: aws.String(vpcid)},
			{ParameterKey: aws.String("Subnets"),
				ParameterValue: aws.String(strings.Join(strings.Split(subnetIds, " "), ","))},
		})
	if err != nil {
		return nil, fmt.Errorf("error creating stack: %v", err)
	}

	nodeInstanceRole := getParameterValueFromOutput("NodeInstanceRole", stack.Stacks[0].Outputs)
	if nodeInstanceRole == "" {
		return nil, fmt.Errorf("no node instance role returned in output: %v", err)
	}

	err = d.createConfigMap(state, *cluster.Cluster.MasterEndpoint, capem, nodeInstanceRole)
	if err != nil {
		return nil, err
	}

	info := &types.ClusterInfo{}
	storeState(info, state)
	return info, nil
}

func isDuplicateKeyError(err error) bool {
	return strings.Contains(err.Error(), "already exists")
}

func isClusterConflict(err error) bool {
	return strings.Contains(err.Error(), "Cluster already exists with name")
}

func getEC2KeyPairName(state state) string {
	return state.ClusterName + "-ec2-key-pair"
}

func getServiceRoleName(state state) string {
	return state.ClusterName + "-eks-service-role"
}

func getVPCStackName(state state) string {
	return state.ClusterName + "-eks-vpc"
}

func (d *Driver) createConfigMap(state state, endpoint string, capem []byte, nodeInstanceRole string) error {
	generator, err := heptio.NewGenerator()
	if err != nil {
		return fmt.Errorf("error creating generator: %v", err)
	}

	token, err := generator.Get(state.ClusterName)
	if err != nil {
		return fmt.Errorf("error generating token: %v", err)
	}

	config := &rest.Config{
		Host: endpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: capem,
		},
		BearerToken: token,
	}

	logrus.Infof("Applying ConfigMap")

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("error creating clientset: %v", err)
	}

	data := []map[string]interface{}{
		{
			"rolearn":  nodeInstanceRole,
			"username": "system:node:{{EC2PrivateDNSName}}",
			"groups": []string{
				"system:bootstrappers",
				"system:nodes",
				"system:node-proxier",
			},
		},
	}
	mapRoles, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshalling map roles: %v", err)
	}

	_, err = clientset.CoreV1().ConfigMaps("default").Create(&v1.ConfigMap{
		TypeMeta: v12.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: v12.ObjectMeta{
			Name:      "aws-auth",
			Namespace: "default",
		},
		Data: map[string]string{
			"mapRoles": string(mapRoles),
		},
	})
	if err != nil && !errors.IsConflict(err) {
		return fmt.Errorf("error creating config map: %v", err)
	}

	return nil
}

func (d *Driver) waitForClusterReady(state state) (*eksCluster, error) {
	cluster := &eksCluster{}

	status := ""
	for status != "ACTIVE" {
		time.Sleep(30 * time.Second)

		logrus.Infof("Waiting for cluster to finish provisioning")

		resp, err := d.awsHTTPRequest(state, "https://eks.us-west-2.amazonaws.com/clusters/"+state.ClusterName,
			"GET", nil)
		if err != nil {
			return nil, fmt.Errorf("error posting cluster: %v", err)
		}

		err = json.Unmarshal(resp, cluster)
		if err != nil {
			return nil, fmt.Errorf("error parsing cluster: %v", err)
		}

		status = *cluster.Cluster.Status
	}

	return cluster, nil
}

func getWorkNodeName(state state) string {
	return state.ClusterName + "-eks-worker-nodes"
}

func storeState(info *types.ClusterInfo, state state) error {
	data, err := json.Marshal(state)

	if err != nil {
		return err
	}

	if info.Metadata == nil {
		info.Metadata = map[string]string{}
	}

	info.Metadata["state"] = string(data)

	return nil
}

func getState(info *types.ClusterInfo) (state, error) {
	state := state{}

	err := json.Unmarshal([]byte(info.Metadata["state"]), &state)
	if err != nil {
		logrus.Errorf("Error encountered while marshalling state: %v", err)
	}

	return state, err
}

func getParameterValueFromOutput(key string, outputs []*cloudformation.Output) string {
	for _, output := range outputs {
		if *output.OutputKey == key {
			return *output.OutputValue
		}
	}

	return ""
}

func (d *Driver) Update(ctx context.Context, info *types.ClusterInfo, options *types.DriverOptions) (*types.ClusterInfo, error) {
	logrus.Infof("Starting update")

	// nothing can be updated so just return

	logrus.Infof("Update complete")
	return info, nil
}

func (d *Driver) PostCheck(ctx context.Context, info *types.ClusterInfo) (*types.ClusterInfo, error) {
	logrus.Infof("Starting post-check")

	state, err := getState(info)
	if err != nil {
		return nil, err
	}

	resp, err := d.awsHTTPRequest(state, "https://eks.us-west-2.amazonaws.com/clusters/"+state.ClusterName,
		"GET", nil)
	if err != nil {
		return nil, fmt.Errorf("error getting cluster: %v", err)
	}

	cluster := &eksCluster{}

	err = json.Unmarshal(resp, &cluster)
	if err != nil {
		return nil, fmt.Errorf("error parsing cluster: %v", err)
	}

	capem, err := base64.StdEncoding.DecodeString(*cluster.Cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, fmt.Errorf("error parsing CA data: %v", err)
	}

	generator, err := heptio.NewGenerator()
	if err != nil {
		return nil, fmt.Errorf("error creating generator: %v", err)
	}

	token, err := generator.Get(state.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("error generating token: %v", err)
	}

	info.Endpoint = *cluster.Cluster.MasterEndpoint
	info.RootCaCertificate = *cluster.Cluster.CertificateAuthority.Data

	config := &rest.Config{
		Host: *cluster.Cluster.MasterEndpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: capem,
		},
		BearerToken: token,
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating clientset: %v", err)
	}

	logrus.Infof("Generating service account token")

	info.ServiceAccountToken, err = util.GenerateServiceAccountToken(clientset)
	if err != nil {
		return nil, fmt.Errorf("error generating service account token: %v", err)
	}

	return info, nil
}

func (d *Driver) Remove(ctx context.Context, info *types.ClusterInfo) error {
	logrus.Infof("Starting delete cluster")

	state, err := getState(info)
	if err != nil {
		return fmt.Errorf("error getting state: %v", err)
	}

	_, err = d.awsHTTPRequest(state, "https://eks.us-west-2.amazonaws.com/clusters/"+state.ClusterName, "DELETE", nil)
	if err != nil && !noClusterFound(err) {
		return fmt.Errorf("error deleting cluster: %v", err)
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"),
		Credentials: credentials.NewStaticCredentials(
			state.ClientID,
			state.ClientSecret,
			"",
		),
	})
	if err != nil {
		return fmt.Errorf("error getting new aws session: %v", err)
	}

	svc := cloudformation.New(sess)

	for _, stackName := range []string{getServiceRoleName(state), getVPCStackName(state), getWorkNodeName(state)} {
		_, err = svc.DeleteStack(&cloudformation.DeleteStackInput{
			StackName: aws.String(stackName),
		})
		if err != nil {
			return fmt.Errorf("error deleting stack: %v", err)
		}
	}

	ec2svc := ec2.New(sess)

	_, err = ec2svc.DeleteKeyPair(&ec2.DeleteKeyPairInput{
		KeyName: aws.String(getEC2KeyPairName(state)),
	})
	if err != nil {
		return fmt.Errorf("error deleting key pair: %v", err)
	}

	return nil
}

func noClusterFound(err error) bool {
	return strings.Contains(err.Error(), "No cluster found for name")
}

func (d *Driver) GetCapabilities(ctx context.Context) (*types.Capabilities, error) {
	return &d.driverCapabilities, nil
}
