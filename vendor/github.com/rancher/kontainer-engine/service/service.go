package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rancher/kontainer-engine/cluster"
	"github.com/rancher/kontainer-engine/drivers"
	"github.com/rancher/kontainer-engine/drivers/aks"
	"github.com/rancher/kontainer-engine/drivers/eks"
	"github.com/rancher/kontainer-engine/drivers/gke"
	"github.com/rancher/kontainer-engine/drivers/import"
	"github.com/rancher/kontainer-engine/types"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"gopkg.in/yaml.v2"
)

var (
	pluginAddress = map[string]string{}
	Drivers       = map[string]types.Driver{}
)

func Start() error {
	for name, driver := range drivers.Drivers {
		Drivers[name] = driver
	}
	return nil
}

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
	case "gke":
		config, err := toMap(c.clusterSpec.GoogleKubernetesEngineConfig, "json")
		if err != nil {
			return driverOptions, err
		}
		data = config
		flatten(data, &driverOptions)
	case "rke":
		config, err := yaml.Marshal(c.clusterSpec.RancherKubernetesEngineConfig)
		if err != nil {
			return driverOptions, err
		}
		driverOptions.StringOptions["rkeConfig"] = string(config)
	case "aks":
		config, err := toMap(c.clusterSpec.AzureKubernetesServiceConfig, "json")
		if err != nil {
			return driverOptions, err
		}
		data = config
		flatten(data, &driverOptions)
	case "import":
		config, err := toMap(c.clusterSpec.ImportedConfig, "json")
		if err != nil {
			return driverOptions, err
		}
		data = config
		flatten(data, &driverOptions)
	case "eks":
		config, err := toMap(c.clusterSpec.AmazonElasticContainerServiceConfig, "json")
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
	Create(ctx context.Context, name string, clusterSpec v3.ClusterSpec) (string, string, string, error)
	Update(ctx context.Context, name string, clusterSpec v3.ClusterSpec) (string, string, string, error)
	Remove(ctx context.Context, name string, clusterSpec v3.ClusterSpec) error
}

type engineService struct {
	store cluster.PersistentStore
}

func NewEngineService(store cluster.PersistentStore) EngineService {
	return &engineService{
		store: store,
	}
}

func (e *engineService) convertCluster(name string, spec v3.ClusterSpec) (cluster.Cluster, error) {
	// todo: decide whether we need a driver field
	driverName := ""
	if spec.AzureKubernetesServiceConfig != nil {
		driverName = "aks"
	} else if spec.GoogleKubernetesEngineConfig != nil {
		driverName = "gke"
	} else if spec.RancherKubernetesEngineConfig != nil {
		driverName = "rke"
	} else if spec.AmazonElasticContainerServiceConfig != nil {
		driverName = "eks"
	} else if spec.ImportedConfig != nil {
		driverName = "import"
	}
	if driverName == "" {
		return cluster.Cluster{}, fmt.Errorf("no driver config found")
	}
	pluginAddr := pluginAddress[driverName]
	configGetter := controllerConfigGetter{
		driverName:  driverName,
		clusterSpec: spec,
		clusterName: name,
	}
	var driver types.Driver
	if _, ok := drivers.Drivers[driverName]; !ok {
		rpcClient, err := types.NewClient(driverName, pluginAddr)
		if err != nil {
			return cluster.Cluster{}, err
		}
		driver = rpcClient
	} else {
		switch driverName {
		case "gke":
			driver = gke.NewDriver()
		case "aks":
			driver = aks.NewDriver()
		case "eks":
			driver = eks.NewDriver()
		case "import":
			driver = kubeimport.NewDriver()
		case "rke":
			driver = drivers.Drivers["rke"]
		}
	}
	clusterPlugin, err := cluster.NewCluster(driverName, name, configGetter, e.store, driver)
	if err != nil {
		return cluster.Cluster{}, err
	}
	return *clusterPlugin, nil
}

// Create creates the stub for cluster manager to call
func (e *engineService) Create(ctx context.Context, name string, clusterSpec v3.ClusterSpec) (string, string, string, error) {
	cls, err := e.convertCluster(name, clusterSpec)
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

// Update creates the stub for cluster manager to call
func (e *engineService) Update(ctx context.Context, name string, clusterSpec v3.ClusterSpec) (string, string, string, error) {
	cls, err := e.convertCluster(name, clusterSpec)
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
func (e *engineService) Remove(ctx context.Context, name string, clusterSpec v3.ClusterSpec) error {
	cls, err := e.convertCluster(name, clusterSpec)
	if err != nil {
		return err
	}
	return cls.Remove(ctx)
}
