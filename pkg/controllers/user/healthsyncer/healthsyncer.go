package healthsyncer

import (
	"time"

	"context"

	"reflect"

	"sort"

	"github.com/pkg/errors"
	"github.com/rancher/norman/condition"
	"github.com/rancher/rancher/pkg/ticker"
	corev1 "github.com/rancher/types/apis/core/v1"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	syncInterval = 15 * time.Second
)

type ClusterControllerLifecycle interface {
	Stop(cluster *v3.Cluster)
}

type HealthSyncer struct {
	ctx               context.Context
	clusterName       string
	clusterLister     v3.ClusterLister
	clusters          v3.ClusterInterface
	componentStatuses corev1.ComponentStatusInterface
	clusterManager    ClusterControllerLifecycle
}

func Register(ctx context.Context, workload *config.UserContext, clusterManager ClusterControllerLifecycle) {
	h := &HealthSyncer{
		ctx:               ctx,
		clusterName:       workload.ClusterName,
		clusterLister:     workload.Management.Management.Clusters("").Controller().Lister(),
		clusters:          workload.Management.Management.Clusters(""),
		componentStatuses: workload.Core.ComponentStatuses(""),
		clusterManager:    clusterManager,
	}

	go h.syncHealth(ctx, syncInterval)
}

func (h *HealthSyncer) syncHealth(ctx context.Context, syncHealth time.Duration) {
	for range ticker.Context(ctx, syncHealth) {
		err := h.updateClusterHealth()
		if err != nil && !apierrors.IsConflict(err) {
			logrus.Error(err)
		}
	}
}

func (h *HealthSyncer) getComponentStatus(cluster *v3.Cluster) error {
	cses, err := h.componentStatuses.List(metav1.ListOptions{})
	if err != nil {
		return condition.Error("ComponentStatsFetchingFailure", errors.Wrap(err, "Failed to communicate with API server"))
	}
	cluster.Status.ComponentStatuses = []v3.ClusterComponentStatus{}
	for _, cs := range cses.Items {
		clusterCS := convertToClusterComponentStatus(&cs)
		cluster.Status.ComponentStatuses = append(cluster.Status.ComponentStatuses, *clusterCS)
	}
	sort.Slice(cluster.Status.ComponentStatuses, func(i, j int) bool {
		return cluster.Status.ComponentStatuses[i].Name < cluster.Status.ComponentStatuses[j].Name
	})
	return nil
}

func (h *HealthSyncer) updateClusterHealth() error {
	oldCluster, err := h.getCluster()
	if err != nil {
		return err
	}
	cluster := oldCluster.DeepCopy()
	if !v3.ClusterConditionProvisioned.IsTrue(cluster) {
		logrus.Debugf("Skip updating cluster health - cluster [%s] not provisioned yet", h.clusterName)
		return nil
	}

	newObj, err := v3.ClusterConditionReady.Do(cluster, func() (runtime.Object, error) {
		for i := 0; ; i++ {
			err := h.getComponentStatus(cluster)
			if err == nil || i > 2 {
				return cluster, err
			}
			select {
			case <-h.ctx.Done():
				return cluster, err
			case <-time.After(5 * time.Second):
			}
		}
	})

	if err == nil {
		v3.ClusterConditionWaiting.True(newObj)
		v3.ClusterConditionWaiting.Message(newObj, "")
	}

	if !reflect.DeepEqual(oldCluster, newObj) {
		if _, err := h.clusters.Update(newObj.(*v3.Cluster)); err != nil {
			return errors.Wrapf(err, "Failed to update cluster [%s]", cluster.Name)
		}
	}

	// Purposefully not return error.  This is so when the cluster goes unavailable we don't just keep failing
	// which will essentially keep the controller alive forever, instead of shutting down.
	return nil
}

func (h *HealthSyncer) getCluster() (*v3.Cluster, error) {
	return h.clusterLister.Get("", h.clusterName)
}

func convertToClusterComponentStatus(cs *v1.ComponentStatus) *v3.ClusterComponentStatus {
	return &v3.ClusterComponentStatus{
		Name:       cs.Name,
		Conditions: cs.Conditions,
	}
}
