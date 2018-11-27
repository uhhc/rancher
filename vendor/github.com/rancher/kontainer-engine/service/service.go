package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/rancher/kontainer-engine/cluster"
	"github.com/rancher/kontainer-engine/drivers/aks"
	"github.com/rancher/kontainer-engine/drivers/eks"
	"github.com/rancher/kontainer-engine/drivers/gke"
	"github.com/rancher/kontainer-engine/drivers/import"
	"github.com/rancher/kontainer-engine/drivers/rke"
	"github.com/rancher/kontainer-engine/types"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/util/rand"
)

var (
	pluginAddress = map[string]string{}
	Drivers       = map[string]types.Driver{
		GoogleKubernetesEngineDriverName:        gke.NewDriver(),
		AzureKubernetesServiceDriverName:        aks.NewDriver(),
		AmazonElasticContainerServiceDriverName: eks.NewDriver(),
		ImportDriverName:                        kubeimport.NewDriver(),
		RancherKubernetesEngineDriverName:       rke.NewDriver(),
	}
)

const (
	ListenAddress                           = "127.0.0.1:"
	GoogleKubernetesEngineDriverName        = "googlekubernetesengine"
	AzureKubernetesServiceDriverName        = "azurekubernetesservice"
	AmazonElasticContainerServiceDriverName = "amazonelasticcontainerservice"
	ImportDriverName                        = "import"
	RancherKubernetesEngineDriverName       = "rancherkubernetesengine"
)

type controllerConfigGetter struct {
	driverName  string
	clusterSpec v3.ClusterSpec
	clusterName string
}

func (c controllerConfigGetter) GetConfig() (types.DriverOptions, error) {
	driverOptions := types.DriverOptions{
		BoolOptions:        make(map[string]bool),
		StringOptions:      make(map[string]string),
		IntOptions:         make(map[string]int64),
		StringSliceOptions: make(map[string]*types.StringSlice),
	}
	data := map[string]interface{}{}
	switch c.driverName {
	case ImportDriverName:
		config, err := toMap(c.clusterSpec.ImportedConfig, "json")
		if err != nil {
			return driverOptions, err
		}
		data = config
		flatten(data, &driverOptions)
	case RancherKubernetesEngineDriverName:
		config, err := yaml.Marshal(c.clusterSpec.RancherKubernetesEngineConfig)
		if err != nil {
			return driverOptions, err
		}
		driverOptions.StringOptions["rkeConfig"] = string(config)
	default:
		config, err := toMap(c.clusterSpec.GenericEngineConfig, "json")
		if err != nil {
			return driverOptions, err
		}
		data = config
		flatten(data, &driverOptions)
	}

	driverOptions.StringOptions["name"] = c.clusterName
	displayName := c.clusterSpec.DisplayName
	if displayName == "" {
		displayName = c.clusterName
	}
	driverOptions.StringOptions["displayName"] = displayName

	return driverOptions, nil
}

// flatten take a map and flatten it and convert it into driverOptions
func flatten(data map[string]interface{}, driverOptions *types.DriverOptions) {
	for k, v := range data {
		switch v.(type) {
		case float64:
			driverOptions.IntOptions[k] = int64(v.(float64))
		case string:
			driverOptions.StringOptions[k] = v.(string)
		case bool:
			driverOptions.BoolOptions[k] = v.(bool)
		case []interface{}:
			// lists of strings come across as lists of interfaces, have to convert them manually
			var stringArray []string

			for _, stringInterface := range v.([]interface{}) {
				switch stringInterface.(type) {
				case string:
					stringArray = append(stringArray, stringInterface.(string))
				}
			}

			// if the length is 0 then it must not have been an array of strings
			if len(stringArray) != 0 {
				driverOptions.StringSliceOptions[k] = &types.StringSlice{Value: stringArray}
			}
		case []string:
			driverOptions.StringSliceOptions[k] = &types.StringSlice{Value: v.([]string)}
		case map[string]interface{}:
			// hack for labels
			if k == "labels" {
				r := []string{}
				for key1, value1 := range v.(map[string]interface{}) {
					r = append(r, fmt.Sprintf("%v=%v", key1, value1))
				}
				driverOptions.StringSliceOptions[k] = &types.StringSlice{Value: r}
			} else {
				flatten(v.(map[string]interface{}), driverOptions)
			}
		default:
			logrus.Warnf("could not convert %v %v=%v", reflect.TypeOf(v), k, v)
		}
	}
}

func toMap(obj interface{}, format string) (map[string]interface{}, error) {
	if format == "json" {
		data, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, err
		}
		return result, nil
	} else if format == "yaml" {
		data, err := yaml.Marshal(obj)
		if err != nil {
			return nil, err
		}
		var result map[string]interface{}
		if err := yaml.Unmarshal(data, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, nil
}

type EngineService interface {
	Create(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) (string, string, string, error)
	Update(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) (string, string, string, error)
	Remove(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) error
	GetDriverCreateOptions(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) (*types.DriverFlags, error)
	GetDriverUpdateOptions(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) (*types.DriverFlags, error)
	GetK8sCapabilities(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) (*types.K8SCapabilities, error)
}

type engineService struct {
	store cluster.PersistentStore
}

func NewEngineService(store cluster.PersistentStore) EngineService {
	return &engineService{
		store: store,
	}
}

func (e *engineService) convertCluster(name string, listenAddr string, spec v3.ClusterSpec) (cluster.Cluster, error) {
	// todo: decide whether we need a driver field
	driverName := ""
	if spec.ImportedConfig != nil {
		driverName = ImportDriverName
	} else if spec.RancherKubernetesEngineConfig != nil {
		driverName = RancherKubernetesEngineDriverName
	} else if spec.GenericEngineConfig != nil {
		driverName = (*spec.GenericEngineConfig)["driverName"].(string)
		if driverName == "" {
			return cluster.Cluster{}, fmt.Errorf("no driver name supplied")
		}
	}
	if driverName == "" {
		return cluster.Cluster{}, fmt.Errorf("no driver config found")
	}

	configGetter := controllerConfigGetter{
		driverName:  driverName,
		clusterSpec: spec,
		clusterName: name,
	}
	clusterPlugin, err := cluster.NewCluster(driverName, name, listenAddr, configGetter, e.store)
	if err != nil {
		return cluster.Cluster{}, err
	}

	// verify driver is running
	failures := 0
	for {
		_, err = clusterPlugin.GetCapabilities(context.Background())
		if err == nil {
			break
		} else if failures > 5 {
			return *clusterPlugin, fmt.Errorf("error checking driver is up: %v", err)
		}

		failures = failures + 1
		time.Sleep(time.Duration(failures*failures) * time.Second)
	}

	return *clusterPlugin, nil
}

// Create creates the stub for cluster manager to call
func (e *engineService) Create(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) (string, string, string, error) {
	runningDriver, err := e.getRunningDriver(kontainerDriver, clusterSpec)
	if err != nil {
		return "", "", "", err
	}

	listenAddr, err := runningDriver.Start()
	if err != nil {
		return "", "", "", fmt.Errorf("error starting driver: %v", err)
	}

	defer runningDriver.Stop()

	cls, err := e.convertCluster(name, listenAddr, clusterSpec)
	if err != nil {
		return "", "", "", err
	}

	if err := cls.Create(ctx); err != nil {
		return "", "", "", err
	}
	endpoint := cls.Endpoint
	if !strings.HasPrefix(endpoint, "https://") {
		endpoint = fmt.Sprintf("https://%s", cls.Endpoint)
	}
	return endpoint, cls.ServiceAccountToken, cls.RootCACert, nil
}

func (e *engineService) getRunningDriver(kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) (*RunningDriver, error) {
	return &RunningDriver{
		Name:    kontainerDriver.Name,
		Builtin: kontainerDriver.Spec.BuiltIn,
		Path:    kontainerDriver.Status.ExecutablePath,
	}, nil
}

// Update creates the stub for cluster manager to call
func (e *engineService) Update(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) (string, string, string, error) {
	runningDriver, err := e.getRunningDriver(kontainerDriver, clusterSpec)
	if err != nil {
		return "", "", "", err
	}

	listenAddr, err := runningDriver.Start()
	if err != nil {
		return "", "", "", fmt.Errorf("error starting driver: %v", err)
	}

	defer runningDriver.Stop()

	cls, err := e.convertCluster(name, listenAddr, clusterSpec)
	if err != nil {
		return "", "", "", err
	}
	if err := cls.Update(ctx); err != nil {
		return "", "", "", err
	}
	endpoint := cls.Endpoint
	if !strings.HasPrefix(endpoint, "https://") {
		endpoint = fmt.Sprintf("https://%s", cls.Endpoint)
	}
	return endpoint, cls.ServiceAccountToken, cls.RootCACert, nil
}

// Remove removes stub for cluster manager to call
func (e *engineService) Remove(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) error {
	runningDriver, err := e.getRunningDriver(kontainerDriver, clusterSpec)
	if err != nil {
		return err
	}

	listenAddr, err := runningDriver.Start()
	if err != nil {
		return fmt.Errorf("error starting driver: %v", err)
	}

	defer runningDriver.Stop()

	cls, err := e.convertCluster(name, listenAddr, clusterSpec)
	if err != nil {
		return err
	}
	return cls.Remove(ctx)
}

func (e *engineService) GetDriverCreateOptions(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) (*types.DriverFlags,
	error) {
	runningDriver, err := e.getRunningDriver(kontainerDriver, clusterSpec)
	if err != nil {
		return nil, err
	}

	listenAddr, err := runningDriver.Start()
	if err != nil {
		return nil, fmt.Errorf("error starting driver: %v", err)
	}

	defer runningDriver.Stop()

	cls, err := e.convertCluster(name, listenAddr, clusterSpec)
	if err != nil {
		return nil, err
	}

	return cls.GetDriverCreateOptions(ctx)
}

func (e *engineService) GetDriverUpdateOptions(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver, clusterSpec v3.ClusterSpec) (*types.DriverFlags,
	error) {
	runningDriver, err := e.getRunningDriver(kontainerDriver, clusterSpec)
	if err != nil {
		return nil, err
	}

	listenAddr, err := runningDriver.Start()
	if err != nil {
		return nil, fmt.Errorf("error starting driver: %v", err)
	}

	defer runningDriver.Stop()

	cls, err := e.convertCluster(name, listenAddr, clusterSpec)
	if err != nil {
		return nil, err
	}

	return cls.GetDriverUpdateOptions(ctx)
}

func (e *engineService) GetK8sCapabilities(ctx context.Context, name string, kontainerDriver *v3.KontainerDriver,
	clusterSpec v3.ClusterSpec) (*types.K8SCapabilities, error) {
	runningDriver, err := e.getRunningDriver(kontainerDriver, clusterSpec)
	if err != nil {
		return nil, err
	}

	listenAddr, err := runningDriver.Start()
	if err != nil {
		return nil, fmt.Errorf("error starting driver: %v", err)
	}

	defer runningDriver.Stop()

	cls, err := e.convertCluster(name, listenAddr, clusterSpec)
	if err != nil {
		return nil, err
	}

	return cls.GetK8SCapabilities(ctx)
}

type RunningDriver struct {
	Name    string
	Path    string
	Builtin bool
	Server  *types.GrpcServer

	listenAddress string
	cancel        context.CancelFunc
}

func (r *RunningDriver) Start() (string, error) {
	port := rand.Intn(1000) + 10*1000 // KontainerDriver port range 10,000 - 11,000
	listenAddress := fmt.Sprintf("%v%v", ListenAddress, port)

	if r.Builtin {
		driver := Drivers[r.Name]
		if driver == nil {
			return "", fmt.Errorf("no driver for name: %v", r.Name)
		}

		addr := make(chan string)
		errChan := make(chan error)
		r.Server = types.NewServer(driver, addr)
		go r.Server.Serve(listenAddress, errChan)

		// if the error hasn't appeared after 5 seconds assume it won't error
		var err error
		select {
		case err = <-errChan:
			// get error
		case <-time.After(5 * time.Second):
			// do nothing
		}
		if err != nil {
			return "", fmt.Errorf("error starting driver: %v", err)
		}

		r.listenAddress = <-addr
	} else {
		var processContext context.Context
		processContext, r.cancel = context.WithCancel(context.Background())

		cmd := exec.CommandContext(processContext, r.Path, strconv.Itoa(port))

		// redirect output to console
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Start()
		if err != nil {
			return "", fmt.Errorf("error starting driver: %v", err)
		}

		r.listenAddress = fmt.Sprintf("127.0.0.1:%v", port)
	}

	logrus.Infof("kontainerdriver %v listening on address %v", r.Name, r.listenAddress)

	return r.listenAddress, nil
}

func (r *RunningDriver) Stop() {
	if r.Builtin {
		r.Server.Stop()
	} else {
		r.cancel()
	}

	logrus.Infof("kontainerdriver %v stopped", r.Name)
}
