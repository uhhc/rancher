package clusterprovisioner

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/rancher/kontainer-engine/drivers/rke"
	"github.com/rancher/kontainer-engine/service"
	"github.com/rancher/norman/controller"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/norman/types/slice"
	"github.com/rancher/rancher/pkg/ref"
	"github.com/rancher/rancher/pkg/rkedialerfactory"
	"github.com/rancher/rancher/pkg/settings"
	"github.com/rancher/rke/services"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/flowcontrol"
)

const (
	RKEDriverKey = "rancherKubernetesEngineConfig"
)

type Provisioner struct {
	ClusterController     v3.ClusterController
	Clusters              v3.ClusterInterface
	NodeLister            v3.NodeLister
	Driver                service.EngineService
	backoff               *flowcontrol.Backoff
	KontainerDriverLister v3.KontainerDriverLister
	DynamicSchemasLister  v3.DynamicSchemaLister
	Backups               v3.EtcdBackupLister
}

func Register(ctx context.Context, management *config.ManagementContext) {
	p := &Provisioner{
		Driver:                service.NewEngineService(NewPersistentStore(management.Core.Namespaces(""), management.Core)),
		Clusters:              management.Management.Clusters(""),
		ClusterController:     management.Management.Clusters("").Controller(),
		NodeLister:            management.Management.Nodes("").Controller().Lister(),
		backoff:               flowcontrol.NewBackOff(30*time.Second, 10*time.Minute),
		KontainerDriverLister: management.Management.KontainerDrivers("").Controller().Lister(),
		DynamicSchemasLister:  management.Management.DynamicSchemas("").Controller().Lister(),
		Backups:               management.Management.EtcdBackups("").Controller().Lister(),
	}

	// Add handlers
	p.Clusters.AddLifecycle(ctx, "cluster-provisioner-controller", p)
	management.Management.Nodes("").AddHandler(ctx, "cluster-provisioner-controller", p.machineChanged)

	local := &rkedialerfactory.RKEDialerFactory{
		Factory: management.Dialer,
	}
	docker := &rkedialerfactory.RKEDialerFactory{
		Factory: management.Dialer,
		Docker:  true,
	}

	driver := service.Drivers[service.RancherKubernetesEngineDriverName]
	rkeDriver := driver.(*rke.Driver)
	rkeDriver.DockerDialer = docker.Build
	rkeDriver.LocalDialer = local.Build
	rkeDriver.WrapTransportFactory = docker.WrapTransport
}

func (p *Provisioner) Remove(cluster *v3.Cluster) (runtime.Object, error) {
	logrus.Infof("Deleting cluster [%s]", cluster.Name)
	if skipProvisioning(cluster) ||
		cluster.Status.Driver == "" {
		return nil, nil
	}

	for i := 0; i < 4; i++ {
		err := p.driverRemove(cluster)
		if err == nil {
			break
		}
		if i == 3 {
			return cluster, fmt.Errorf("failed to remove the cluster [%s]: %v", cluster.Name, err)
		}
		time.Sleep(1 * time.Second)
	}
	logrus.Infof("Deleted cluster [%s]", cluster.Name)

	// cluster object will definitely have changed, reload
	return p.Clusters.Get(cluster.Name, metav1.GetOptions{})
}

func (p *Provisioner) Updated(cluster *v3.Cluster) (runtime.Object, error) {
	obj, err := v3.ClusterConditionUpdated.Do(cluster, func() (runtime.Object, error) {
		setVersion(cluster)
		return p.update(cluster, false)
	})

	return obj.(*v3.Cluster), err
}

func setVersion(cluster *v3.Cluster) {
	if cluster.Spec.RancherKubernetesEngineConfig != nil {
		if cluster.Spec.RancherKubernetesEngineConfig.Version == "" {
			//set version from the applied spec
			if cluster.Status.AppliedSpec.RancherKubernetesEngineConfig != nil {
				if cluster.Status.AppliedSpec.RancherKubernetesEngineConfig.Version != "" {
					cluster.Spec.RancherKubernetesEngineConfig.Version = cluster.Status.AppliedSpec.RancherKubernetesEngineConfig.Version
				} else {
					cluster.Spec.RancherKubernetesEngineConfig.Version = settings.KubernetesVersion.Get()
				}
			}
		}
	}
}

func (p *Provisioner) update(cluster *v3.Cluster, create bool) (*v3.Cluster, error) {
	cluster, err := p.reconcileCluster(cluster, create)
	if err != nil {
		return cluster, err
	}

	v3.ClusterConditionProvisioned.True(cluster)
	v3.ClusterConditionProvisioned.Message(cluster, "")
	v3.ClusterConditionProvisioned.Reason(cluster, "")
	v3.ClusterConditionPending.True(cluster)
	return cluster, nil
}

func (p *Provisioner) machineChanged(key string, machine *v3.Node) (runtime.Object, error) {
	parts := strings.SplitN(key, "/", 2)

	p.ClusterController.Enqueue("", parts[0])

	return machine, nil
}

func (p *Provisioner) Create(cluster *v3.Cluster) (runtime.Object, error) {
	var err error

	// Initialize conditions, be careful to not continually update them
	v3.ClusterConditionPending.CreateUnknownIfNotExists(cluster)
	v3.ClusterConditionProvisioned.CreateUnknownIfNotExists(cluster)

	if v3.ClusterConditionWaiting.GetStatus(cluster) == "" {
		v3.ClusterConditionWaiting.Unknown(cluster)
	}
	if v3.ClusterConditionWaiting.GetMessage(cluster) == "" {
		v3.ClusterConditionWaiting.Message(cluster, "Waiting for API to be available")
	}

	cluster, err = p.pending(cluster)
	if err != nil {
		return cluster, err
	}

	return p.provision(cluster)
}

func (p *Provisioner) provision(cluster *v3.Cluster) (*v3.Cluster, error) {
	obj, err := v3.ClusterConditionProvisioned.Do(cluster, func() (runtime.Object, error) {
		return p.update(cluster, true)
	})
	return obj.(*v3.Cluster), err
}

func (p *Provisioner) pending(cluster *v3.Cluster) (*v3.Cluster, error) {
	if skipProvisioning(cluster) {
		return cluster, nil
	}

	driver, err := p.validateDriver(cluster)
	if err != nil {
		return cluster, err
	}

	if driver == "" {
		return cluster, &controller.ForgetError{
			Err:    fmt.Errorf("waiting for full cluster configuration"),
			Reason: "Pending"}
	}

	if driver != cluster.Status.Driver {
		cluster.Status.Driver = driver
		if driver == v3.ClusterDriverRKE && cluster.Spec.RancherKubernetesEngineConfig == nil {
			cluster.Spec.RancherKubernetesEngineConfig = &v3.RancherKubernetesEngineConfig{}
		}
		return p.Clusters.Update(cluster)
	}

	return cluster, nil

}

func (p *Provisioner) backoffFailure(cluster *v3.Cluster, spec *v3.ClusterSpec) (bool, time.Duration) {
	if cluster.Status.FailedSpec == nil {
		return false, 0
	}

	if !reflect.DeepEqual(cluster.Status.FailedSpec, spec) {
		return false, 0
	}

	if p.backoff.IsInBackOffSinceUpdate(cluster.Name, time.Now()) {
		go func() {
			time.Sleep(p.backoff.Get(cluster.Name))
			p.ClusterController.Enqueue("", cluster.Name)
		}()
		return true, p.backoff.Get(cluster.Name)
	}

	return false, 0
}

// reconcileCluster returns true if waiting or false if ready to provision
func (p *Provisioner) reconcileCluster(cluster *v3.Cluster, create bool) (*v3.Cluster, error) {
	if skipProvisioning(cluster) {
		return cluster, nil
	}

	var (
		apiEndpoint, serviceAccountToken, caCert string
		err                                      error
	)

	if cluster.Name != "local" && !v3.ClusterConditionServiceAccountMigrated.IsTrue(cluster) &&
		v3.ClusterConditionProvisioned.IsTrue(cluster) {
		driverName, err := p.validateDriver(cluster)
		if err != nil {
			return nil, err
		}

		spec, _, err := p.getConfig(true, cluster.Spec, driverName, cluster.Name)
		if err != nil {
			return nil, err
		}

		serviceAccountToken, err = p.generateServiceAccount(cluster, *spec)
		if err != nil {
			return nil, err
		}

		cluster.Status.ServiceAccountToken = serviceAccountToken
		v3.ClusterConditionServiceAccountMigrated.True(cluster)

		// Update the cluster in k8s
		cluster, err = p.Clusters.Update(cluster)
		if err != nil {
			return nil, err
		}

		err = p.removeLegacyServiceAccount(cluster, *spec)
		if err != nil {
			return nil, err
		}
	}

	spec, err := p.getSpec(cluster)
	if err != nil || spec == nil {
		return cluster, err
	}

	if ok, delay := p.backoffFailure(cluster, spec); ok {
		return cluster, &controller.ForgetError{Err: fmt.Errorf("backing off failure, delay: %v", delay)}
	}

	logrus.Infof("Provisioning cluster [%s]", cluster.Name)

	if create {
		logrus.Infof("Creating cluster [%s]", cluster.Name)
		apiEndpoint, serviceAccountToken, caCert, err = p.driverCreate(cluster, *spec)
		if err != nil && err.Error() == "cluster already exists" {
			logrus.Infof("Create done, Updating cluster [%s]", cluster.Name)
			apiEndpoint, serviceAccountToken, caCert, err = p.driverUpdate(cluster, *spec)
		}
	} else if spec.RancherKubernetesEngineConfig != nil && spec.RancherKubernetesEngineConfig.Restore.Restore {
		logrus.Infof("Restoring cluster [%s] from backup", cluster.Name)
		apiEndpoint, serviceAccountToken, caCert, err = p.restoreClusterBackup(cluster, *spec)
	} else if spec.RancherKubernetesEngineConfig != nil && spec.RancherKubernetesEngineConfig.RotateCertificates != nil {
		logrus.Infof("Rotating certificates for cluster [%s]", cluster.Name)
		apiEndpoint, serviceAccountToken, caCert, err = p.driverUpdate(cluster, *spec)
	} else {
		logrus.Infof("Updating cluster [%s]", cluster.Name)
		apiEndpoint, serviceAccountToken, caCert, err = p.driverUpdate(cluster, *spec)
	}
	// at this point we know the cluster has been modified in driverCreate/Update so reload
	if newCluster, reloadErr := p.Clusters.Get(cluster.Name, metav1.GetOptions{}); reloadErr == nil {
		cluster = newCluster
	}

	cluster, recordErr := p.recordFailure(cluster, *spec, err)
	if recordErr != nil {
		return cluster, recordErr
	}

	// for here out we want to always return the cluster, not just nil, so that the error can be properly
	// recorded if needs be
	if err != nil {
		return cluster, err
	}

	err = p.removeLegacyServiceAccount(cluster, *spec)
	if err != nil {
		return nil, err
	}

	v3.ClusterConditionServiceAccountMigrated.True(cluster)

	saved := false
	for i := 0; i < 20; i++ {
		cluster, err = p.Clusters.Get(cluster.Name, metav1.GetOptions{})
		if err != nil {
			return cluster, err
		}

		censoredSpec, err := p.censorGenericEngineConfig(*spec)
		if err != nil {
			return cluster, err
		}

		cluster.Status.AppliedSpec = censoredSpec
		cluster.Status.APIEndpoint = apiEndpoint
		cluster.Status.ServiceAccountToken = serviceAccountToken
		cluster.Status.CACert = caCert
		resetRkeConfigFlags(cluster)

		if cluster, err = p.Clusters.Update(cluster); err == nil {
			saved = true
			break
		} else {
			logrus.Errorf("failed to update cluster [%s]: %v", cluster.Name, err)
			time.Sleep(2)
		}
	}

	if !saved {
		return cluster, fmt.Errorf("failed to update cluster")
	}

	logrus.Infof("Provisioned cluster [%s]", cluster.Name)
	return cluster, nil
}

func resetRkeConfigFlags(cluster *v3.Cluster) {
	if cluster.Spec.RancherKubernetesEngineConfig != nil {
		cluster.Spec.RancherKubernetesEngineConfig.RotateCertificates = nil
		cluster.Spec.RancherKubernetesEngineConfig.Restore = v3.RestoreConfig{}
		if cluster.Status.AppliedSpec.RancherKubernetesEngineConfig != nil {
			cluster.Status.AppliedSpec.RancherKubernetesEngineConfig.RotateCertificates = nil
			cluster.Status.AppliedSpec.RancherKubernetesEngineConfig.Restore = v3.RestoreConfig{}
		}
	}
}

func copyMap(toCopy v3.MapStringInterface) v3.MapStringInterface {
	newMap := v3.MapStringInterface{}

	for k, v := range toCopy {
		newMap[k] = v
	}

	return newMap
}

func (p *Provisioner) censorGenericEngineConfig(input v3.ClusterSpec) (v3.ClusterSpec, error) {
	if input.GenericEngineConfig == nil {
		// nothing to do
		return input, nil
	}

	config := copyMap(*input.GenericEngineConfig)
	driverName, ok := config[DriverNameField].(string)
	if !ok {
		// can't figure out driver type so blank out the whole thing
		logrus.Warnf("cluster %v has a generic engine config but no driver type field; can't hide password "+
			"fields so removing the entire config", input.DisplayName)
		input.GenericEngineConfig = nil
		return input, nil
	}

	driver, err := p.KontainerDriverLister.Get("", driverName)
	if err != nil {
		return v3.ClusterSpec{}, err
	}

	var schemaName string
	if driver.Spec.BuiltIn {
		schemaName = driver.Status.DisplayName + "Config"
	} else {
		schemaName = driver.Status.DisplayName + "EngineConfig"
	}

	kontainerDriverSchema, err := p.DynamicSchemasLister.Get("", strings.ToLower(schemaName))
	if err != nil {
		return v3.ClusterSpec{}, fmt.Errorf("error getting dynamic schema %v", err)
	}

	for key := range config {
		field := kontainerDriverSchema.Spec.ResourceFields[key]
		if field.Type == "password" {
			delete(config, key)
		}
	}

	input.GenericEngineConfig = &config
	return input, nil
}

func skipProvisioning(cluster *v3.Cluster) bool {
	return cluster.Status.Driver == v3.ClusterDriverLocal || cluster.Status.Driver == v3.ClusterDriverImported
}

func (p *Provisioner) getConfig(reconcileRKE bool, spec v3.ClusterSpec, driverName, clusterName string) (*v3.ClusterSpec, interface{}, error) {
	var v interface{}
	if spec.GenericEngineConfig == nil {
		if spec.RancherKubernetesEngineConfig != nil {
			var err error
			v, err = convert.EncodeToMap(spec.RancherKubernetesEngineConfig)
			if err != nil {
				return nil, nil, err
			}
		} else {
			v = map[string]interface{}{}
		}
	} else {
		v = *spec.GenericEngineConfig
	}

	if driverName == v3.ClusterDriverRKE && reconcileRKE {
		nodes, err := p.reconcileRKENodes(clusterName)
		if err != nil {
			return nil, nil, err
		}

		systemImages, err := getSystemImages(spec)
		if err != nil {
			return nil, nil, err
		}

		copy := *spec.RancherKubernetesEngineConfig
		spec.RancherKubernetesEngineConfig = &copy
		spec.RancherKubernetesEngineConfig.Nodes = nodes
		spec.RancherKubernetesEngineConfig.SystemImages = *systemImages

		data, _ := convert.EncodeToMap(spec)
		v, _ = data[RKEDriverKey]
	}

	return &spec, v, nil
}

func (p *Provisioner) getDriver(cluster *v3.Cluster) (string, error) {
	var driver *v3.KontainerDriver
	var err error

	if cluster.Spec.GenericEngineConfig == nil {
		if cluster.Spec.AmazonElasticContainerServiceConfig != nil {
			cluster.Spec.GenericEngineConfig = cluster.Spec.AmazonElasticContainerServiceConfig
			(*cluster.Spec.GenericEngineConfig)["driverName"] = "amazonelasticcontainerservice"
		}

		if cluster.Spec.AzureKubernetesServiceConfig != nil {
			cluster.Spec.GenericEngineConfig = cluster.Spec.AzureKubernetesServiceConfig
			(*cluster.Spec.GenericEngineConfig)["driverName"] = "azurekubernetesservice"
		}

		if cluster.Spec.GoogleKubernetesEngineConfig != nil {
			cluster.Spec.GenericEngineConfig = cluster.Spec.GoogleKubernetesEngineConfig
			(*cluster.Spec.GenericEngineConfig)["driverName"] = "googlekubernetesengine"
		}
	}

	if cluster.Spec.GenericEngineConfig != nil {
		kontainerDriverName := (*cluster.Spec.GenericEngineConfig)["driverName"].(string)
		driver, err = p.KontainerDriverLister.Get("", kontainerDriverName)
		if err != nil {
			return "", err
		}
	}

	if cluster.Spec.RancherKubernetesEngineConfig != nil {
		return v3.ClusterDriverRKE, nil
	}

	if driver == nil {
		return "", nil
	}

	return driver.Status.DisplayName, nil
}

func (p *Provisioner) validateDriver(cluster *v3.Cluster) (string, error) {
	oldDriver := cluster.Status.Driver

	if oldDriver == v3.ClusterDriverImported {
		return v3.ClusterDriverImported, nil
	}

	newDriver, err := p.getDriver(cluster)
	if err != nil {
		return "", err
	}

	if oldDriver == "" && newDriver == "" {
		return newDriver, nil
	}

	if oldDriver == "" {
		return newDriver, nil
	}

	if newDriver == "" {
		return "", &controller.ForgetError{Err: fmt.Errorf("waiting for nodes")}
	}

	if oldDriver != newDriver {
		return newDriver, fmt.Errorf("driver change from %s to %s not allowed", oldDriver, newDriver)
	}

	return newDriver, nil
}

func getSystemImages(spec v3.ClusterSpec) (*v3.RKESystemImages, error) {
	// fetch system images from settings
	version := spec.RancherKubernetesEngineConfig.Version
	systemImages, ok := v3.AllK8sVersions[version]
	if !ok {
		return nil, fmt.Errorf("failed to find system images for version %v", version)
	}

	privateRegistry := getPrivateRepo(spec.RancherKubernetesEngineConfig)
	if privateRegistry == "" {
		return &systemImages, nil
	}

	// prepend private repo
	imagesMap, err := convert.EncodeToMap(systemImages)
	if err != nil {
		return nil, err
	}
	updatedMap := make(map[string]interface{})
	for key, value := range imagesMap {
		newValue := fmt.Sprintf("%s/%s", privateRegistry, value)
		updatedMap[key] = newValue
	}
	if err := mapstructure.Decode(updatedMap, &systemImages); err != nil {
		return nil, err
	}
	return &systemImages, nil
}

func getPrivateRepo(config *v3.RancherKubernetesEngineConfig) string {
	if len(config.PrivateRegistries) > 0 {
		return config.PrivateRegistries[0].URL
	}
	return settings.SystemDefaultRegistry.Get()
}

func (p *Provisioner) getSpec(cluster *v3.Cluster) (*v3.ClusterSpec, error) {
	driverName, err := p.validateDriver(cluster)
	if err != nil {
		return nil, err
	}

	_, oldConfig, err := p.getConfig(false, cluster.Status.AppliedSpec, driverName, cluster.Name)
	if err != nil {
		return nil, err
	}

	censoredSpec, err := p.censorGenericEngineConfig(cluster.Spec)
	if err != nil {
		return nil, err
	}

	newSpec, newConfig, err := p.getConfig(true, censoredSpec, driverName, cluster.Name)
	if err != nil {
		return nil, err
	}

	if reflect.DeepEqual(oldConfig, newConfig) {
		return nil, nil
	}

	newSpec, _, err = p.getConfig(true, cluster.Spec, driverName, cluster.Name)

	return newSpec, err
}

func (p *Provisioner) reconcileRKENodes(clusterName string) ([]v3.RKEConfigNode, error) {
	machines, err := p.NodeLister.List(clusterName, labels.Everything())
	if err != nil {
		return nil, err
	}

	etcd := false
	controlplane := false
	var nodes []v3.RKEConfigNode
	for _, machine := range machines {
		if machine.DeletionTimestamp != nil {
			continue
		}

		if v3.NodeConditionProvisioned.IsUnknown(machine) && (machine.Spec.Etcd || machine.Spec.ControlPlane) {
			return nil, &controller.ForgetError{
				Err:    fmt.Errorf("waiting for %s to finish provisioning", machine.Spec.RequestedHostname),
				Reason: "Provisioning",
			}
		}

		if machine.Status.NodeConfig == nil {
			continue
		}

		if len(machine.Status.NodeConfig.Role) == 0 {
			continue
		}

		if !v3.NodeConditionProvisioned.IsTrue(machine) {
			continue
		}

		if slice.ContainsString(machine.Status.NodeConfig.Role, services.ETCDRole) {
			etcd = true
		}
		if slice.ContainsString(machine.Status.NodeConfig.Role, services.ControlRole) {
			controlplane = true
		}

		node := *machine.Status.NodeConfig
		if node.User == "" {
			node.User = "root"
		}
		if node.Port == "" {
			node.Port = "22"
		}
		if node.NodeName == "" {
			node.NodeName = ref.FromStrings(machine.Namespace, machine.Name)
		}
		nodes = append(nodes, node)
	}

	if !etcd || !controlplane {
		return nil, &controller.ForgetError{
			Err:    fmt.Errorf("waiting for etcd and controlplane nodes to be registered"),
			Reason: "Provisioning",
		}
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeName < nodes[j].NodeName
	})

	return nodes, nil
}

func (p *Provisioner) recordFailure(cluster *v3.Cluster, spec v3.ClusterSpec, err error) (*v3.Cluster, error) {
	if err == nil {
		p.backoff.DeleteEntry(cluster.Name)
		if cluster.Status.FailedSpec == nil {
			return cluster, nil
		}

		cluster.Status.FailedSpec = nil
		return p.Clusters.Update(cluster)
	}

	p.backoff.Next(cluster.Name, time.Now())
	cluster.Status.FailedSpec = &spec
	newCluster, _ := p.Clusters.Update(cluster)
	// mask the error
	return newCluster, nil
}

func (p *Provisioner) restoreClusterBackup(cluster *v3.Cluster, spec v3.ClusterSpec) (api string, token string, cert string, err error) {
	snapshot := strings.Split(spec.RancherKubernetesEngineConfig.Restore.SnapshotName, ":")[1]
	backup, err := p.Backups.Get(cluster.Name, snapshot)
	if err != nil {
		return "", "", "", err
	}
	if backup.Spec.ClusterID != cluster.Name {
		return "", "", "", fmt.Errorf("snapshot [%s] is not a backup of cluster [%s]", backup.Name, cluster.Name)
	}
	return p.driverRestore(cluster, spec)
}
