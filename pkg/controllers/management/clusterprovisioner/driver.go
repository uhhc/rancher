package clusterprovisioner

import (
	"fmt"
	"reflect"

	v32 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"

	rketypes "github.com/rancher/rke/types"

	"github.com/rancher/rancher/pkg/clusterprovisioninglogger"
	v3 "github.com/rancher/rancher/pkg/generated/norman/management.cattle.io/v3"
	"github.com/rancher/rancher/pkg/kontainer-engine/service"
	"github.com/rancher/rke/services"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

const DriverNameField = "driverName"

func (p *Provisioner) driverCreate(cluster *v3.Cluster, spec v32.ClusterSpec) (api string, token string, cert string, err error) {
	ctx, logger := clusterprovisioninglogger.NewLogger(p.Clusters, cluster, v32.ClusterConditionProvisioned)
	defer logger.Close()

	spec = cleanRKE(spec)

	if newCluster, err := p.Clusters.Update(cluster); err == nil {
		cluster = newCluster
	}

	kontainerDriver, err := p.getKontainerDriver(spec)
	if err != nil {
		return "", "", "", err
	}

	return p.engineService.Create(ctx, cluster.Name, kontainerDriver, spec)
}

func (p *Provisioner) getKontainerDriver(spec v32.ClusterSpec) (*v3.KontainerDriver, error) {
	if spec.GenericEngineConfig != nil {
		return p.KontainerDriverLister.Get("", (*spec.GenericEngineConfig)[DriverNameField].(string))
	}

	if spec.RancherKubernetesEngineConfig != nil {
		return p.KontainerDriverLister.Get("", service.RancherKubernetesEngineDriverName)
	}

	if spec.ImportedConfig != nil {
		return p.KontainerDriverLister.Get("", "import")
	}

	return nil, fmt.Errorf("no kontainer driver for cluster %v", spec.DisplayName)
}

func (p *Provisioner) driverUpdate(cluster *v3.Cluster, spec v32.ClusterSpec) (api string, token string, cert string, updateTriggered bool, err error) {
	ctx, logger := clusterprovisioninglogger.NewLogger(p.Clusters, cluster, v32.ClusterConditionUpdated)
	defer logger.Close()

	spec = cleanRKE(spec)
	applied := cleanRKE(cluster.Status.AppliedSpec)

	if spec.RancherKubernetesEngineConfig != nil && cluster.Status.APIEndpoint != "" && cluster.Status.ServiceAccountToken != "" &&
		reflect.DeepEqual(applied.RancherKubernetesEngineConfig, spec.RancherKubernetesEngineConfig) {
		return cluster.Status.APIEndpoint, cluster.Status.ServiceAccountToken, cluster.Status.CACert, false, nil
	}

	if spec.RancherKubernetesEngineConfig != nil && spec.RancherKubernetesEngineConfig.Services.Etcd.Snapshot == nil &&
		applied.RancherKubernetesEngineConfig != nil && applied.RancherKubernetesEngineConfig.Services.Etcd.Snapshot == nil {
		_false := false
		cluster.Spec.RancherKubernetesEngineConfig.Services.Etcd.Snapshot = &_false
	}

	if newCluster, err := p.Clusters.Update(cluster); err == nil {
		cluster = newCluster
	}

	kontainerDriver, err := p.getKontainerDriver(spec)
	if err != nil {
		return "", "", "", false, err
	}

	api, token, cert, err = p.engineService.Update(ctx, cluster.Name, kontainerDriver, spec)
	return api, token, cert, true, err
}

func (p *Provisioner) driverRemove(cluster *v3.Cluster, forceRemove bool) error {
	ctx, logger := clusterprovisioninglogger.NewLogger(p.Clusters, cluster, v32.ClusterConditionProvisioned)
	defer logger.Close()

	spec := cleanRKE(cluster.Spec)

	_, err := v32.ClusterConditionUpdated.Do(cluster, func() (runtime.Object, error) {
		if newCluster, err := p.Clusters.Update(cluster); err == nil {
			cluster = newCluster
		}

		kontainerDriver, err := p.getKontainerDriver(spec)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrus.Warnf("Could not find kontainer driver for cluster removal [%v]", err)
				return nil, nil
			}
			return nil, err
		}

		return cluster, p.engineService.Remove(ctx, cluster.Name, kontainerDriver, spec, forceRemove)
	})

	return err
}

func (p *Provisioner) driverRestore(cluster *v3.Cluster, spec v32.ClusterSpec, snapshot string) (string, string, string, error) {
	ctx, logger := clusterprovisioninglogger.NewLogger(p.Clusters, cluster, v32.ClusterConditionUpdated)
	defer logger.Close()

	spec = cleanRKE(spec)

	newCluster, err := p.Clusters.Update(cluster)
	if err != nil {
		return "", "", "", err
	}
	cluster = newCluster

	kontainerDriver, err := p.getKontainerDriver(spec)
	if err != nil {
		return "", "", "", err
	}
	return p.engineService.ETCDRestore(ctx, cluster.Name, kontainerDriver, spec, snapshot)

}

func (p *Provisioner) generateServiceAccount(cluster *v3.Cluster, spec v32.ClusterSpec) (string, error) {
	ctx, logger := clusterprovisioninglogger.NewLogger(p.Clusters, cluster, v32.ClusterConditionUpdated)
	defer logger.Close()

	spec = cleanRKE(spec)

	kontainerDriver, err := p.getKontainerDriver(spec)
	if err != nil {
		return "", err
	}

	return p.engineService.GenerateServiceAccount(ctx, cluster.Name, kontainerDriver, spec)
}

func (p *Provisioner) removeLegacyServiceAccount(cluster *v3.Cluster, spec v32.ClusterSpec) error {
	ctx, logger := clusterprovisioninglogger.NewLogger(p.Clusters, cluster, v32.ClusterConditionUpdated)
	defer logger.Close()

	spec = cleanRKE(spec)

	kontainerDriver, err := p.getKontainerDriver(spec)
	if err != nil {
		return err
	}

	return p.engineService.RemoveLegacyServiceAccount(ctx, cluster.Name, kontainerDriver, spec)
}

func cleanRKE(spec v32.ClusterSpec) v32.ClusterSpec {
	if spec.RancherKubernetesEngineConfig == nil {
		return spec
	}

	result := spec.DeepCopy()

	var filteredNodes []rketypes.RKEConfigNode
	for _, node := range spec.RancherKubernetesEngineConfig.Nodes {
		if len(node.Role) == 1 && node.Role[0] == services.WorkerRole {
			continue
		}
		filteredNodes = append(filteredNodes, node)
	}

	result.RancherKubernetesEngineConfig.Nodes = filteredNodes
	return *result
}
