package librke

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/rke/cluster"
	"github.com/rancher/rke/hosts"
	"github.com/rancher/rke/k8s"
	"github.com/rancher/rke/pki"
	"github.com/rancher/types/apis/management.cattle.io/v3"
)

type rke struct {
}

func (*rke) GenerateRKENodeCerts(ctx context.Context, rkeConfig v3.RancherKubernetesEngineConfig, nodeAddress string, certBundle map[string]pki.CertificatePKI) map[string]pki.CertificatePKI {
	return pki.GenerateRKENodeCerts(ctx, rkeConfig, nodeAddress, certBundle)
}

func (*rke) GenerateCerts(config *v3.RancherKubernetesEngineConfig) (map[string]pki.CertificatePKI, error) {
	return pki.GenerateRKECerts(context.Background(), *config, "", "")
}

func (*rke) RegenerateEtcdCertificate(crtMap map[string]pki.CertificatePKI, etcdHost *hosts.Host, cluster *cluster.Cluster) (map[string]pki.CertificatePKI, error) {
	return pki.RegenerateEtcdCertificate(context.Background(),
		crtMap,
		etcdHost,
		cluster.EtcdHosts,
		cluster.ClusterDomain,
		cluster.KubernetesServiceIP)
}

func (*rke) ParseCluster(clusterName string, config *v3.RancherKubernetesEngineConfig,
	dockerDialerFactory,
	localConnDialerFactory hosts.DialerFactory,
	k8sWrapTransport k8s.WrapTransport) (*cluster.Cluster, error) {

	clusterFilePath := clusterName + "-cluster.yaml"
	if clusterName == "local" {
		clusterFilePath = ""
	}

	return cluster.ParseCluster(context.Background(),
		config, clusterFilePath, "",
		dockerDialerFactory,
		localConnDialerFactory,
		k8sWrapTransport)
}

func (*rke) GeneratePlan(ctx context.Context, rkeConfig *v3.RancherKubernetesEngineConfig, dockerInfo map[string]types.Info) (v3.RKEPlan, error) {
	return cluster.GeneratePlan(ctx, rkeConfig.DeepCopy(), dockerInfo)
}

func GetDockerInfo(node *v3.Node) (map[string]types.Info, error) {
	infos := map[string]types.Info{}
	if node.Status.DockerInfo != nil {
		dockerInfo := types.Info{}
		err := convert.ToObj(node.Status.DockerInfo, &dockerInfo)
		if err != nil {
			return nil, err
		}
		infos[node.Status.NodeConfig.Address] = dockerInfo
	}

	return infos, nil
}
