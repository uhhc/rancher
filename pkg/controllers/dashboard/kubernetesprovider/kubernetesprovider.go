package kubernetesprovider

import (
	"context"
	"errors"

	detector "github.com/rancher/kubernetes-provider-detector"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	v32 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher/pkg/wrangler"
	"k8s.io/client-go/kubernetes"
)

type handler struct {
	ctx                context.Context
	clusters           v32.ClusterClient
	localClusterClient kubernetes.Interface
	mcm                wrangler.MultiClusterManager
}

func Register(ctx context.Context,
	clusters v32.ClusterController,
	localClusterClient kubernetes.Interface,
	mcm wrangler.MultiClusterManager,
) {
	h := &handler{
		ctx:                ctx,
		clusters:           clusters,
		localClusterClient: localClusterClient,
		mcm:                mcm,
	}
	clusters.OnChange(ctx, "kubernetes-provider", h.OnChange)
}

func (h *handler) OnChange(key string, cluster *v3.Cluster) (*v3.Cluster, error) {
	if cluster == nil || cluster.Status.Provider != "" {
		return cluster, nil
	}

	var client kubernetes.Interface
	if cluster.Spec.Internal {
		client = h.localClusterClient
	} else if k8s, err := h.mcm.K8sClient(cluster.Name); err != nil {
		return nil, err
	} else if k8s != nil {
		client = k8s
	}

	if client == nil {
		return cluster, nil
	}

	provider, err := detector.DetectProvider(h.ctx, client)
	var u detector.ErrUnknownProvider
	if errors.Is(err, &u) {
		return cluster, nil
	} else if err != nil {
		return cluster, err
	}
	cluster = cluster.DeepCopy()
	cluster.Status.Provider = provider
	return h.clusters.Update(cluster)
}
