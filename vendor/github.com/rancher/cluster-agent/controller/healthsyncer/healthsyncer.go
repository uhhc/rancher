package healthsyncer

import (
	"fmt"
	"time"

	"context"

	"github.com/pkg/errors"
	"github.com/rancher/cluster-agent/utils"
	"github.com/rancher/norman/condition"
	corev1 "github.com/rancher/types/apis/core/v1"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	syncInterval = 15 * time.Second
)

type HealthSyncer struct {
	clusterName       string
	Clusters          v3.ClusterInterface
	ComponentStatuses corev1.ComponentStatusInterface
}

func Register(ctx context.Context, workload *config.ClusterContext) {
	h := &HealthSyncer{
		clusterName:       workload.ClusterName,
		Clusters:          workload.Management.Management.Clusters(""),
		ComponentStatuses: workload.Core.ComponentStatuses(""),
	}

	go h.syncHealth(ctx, syncInterval)
}

func (h *HealthSyncer) syncHealth(ctx context.Context, syncHealth time.Duration) {
	for range utils.TickerContext(ctx, syncHealth) {
		err := h.updateClusterHealth()
		if err != nil {
			logrus.Info(err)
		}
	}
}

func (h *HealthSyncer) updateClusterHealth() error {
	cluster, err := h.getCluster()
	if err != nil {
		return err
	}
	if cluster == nil {
		logrus.Debugf("Skip updating cluster health, cluster [%s] deleted", h.clusterName)
		return nil
	}
	if !v3.ClusterConditionProvisioned.IsTrue(cluster) {
		logrus.Debugf("Skip updating cluster health - cluster [%s] not provisioned yet", h.clusterName)
		return nil
	}

	newObj, err := v3.ClusterConditionReady.Do(cluster, func() (runtime.Object, error) {
		cses, err := h.ComponentStatuses.List(metav1.ListOptions{})
		if err != nil {
			return cluster, condition.Error("ComponentStatsFetchingFailure", errors.Wrap(err, "Failed to communicate with API server"))
		}
		cluster.Status.ComponentStatuses = []v3.ClusterComponentStatus{}
		for _, cs := range cses.Items {
			clusterCS := convertToClusterComponentStatus(&cs)
			cluster.Status.ComponentStatuses = append(cluster.Status.ComponentStatuses, *clusterCS)
		}
		return cluster, nil
	})

	_, err = h.Clusters.Update(newObj.(*v3.Cluster))
	if err != nil {
		return fmt.Errorf("Failed to update cluster [%s] %v", cluster.Name, err)
	}

	logrus.Debugf("Updated cluster health successfully [%s]", h.clusterName)
	return nil
}

func (h *HealthSyncer) getCluster() (*v3.Cluster, error) {
	return h.Clusters.Get(h.clusterName, metav1.GetOptions{})
}

func convertToClusterComponentStatus(cs *v1.ComponentStatus) *v3.ClusterComponentStatus {
	return &v3.ClusterComponentStatus{
		Name:       cs.Name,
		Conditions: cs.Conditions,
	}
}
