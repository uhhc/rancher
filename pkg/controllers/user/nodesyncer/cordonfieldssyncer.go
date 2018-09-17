package nodesyncer

import (
	"fmt"

	"strings"

	"context"

	"sync"

	"time"

	"github.com/rancher/norman/types/convert"
	"github.com/rancher/rancher/pkg/kubectl"
	nodehelper "github.com/rancher/rancher/pkg/node"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	drainTokenPrefix = "drain-node-"
	description      = "token for drain"
)

var nodeMapLock = sync.Mutex{}
var toIgnoreErrs = []string{"--ignore-daemonsets", "--delete-local-data", "--force", "did not complete within"}

func (m *NodesSyncer) syncCordonFields(key string, obj *v3.Node) error {
	if obj == nil || obj.DeletionTimestamp != nil || obj.Spec.DesiredNodeUnschedulable == "" {
		return nil
	}

	if obj.Spec.DesiredNodeUnschedulable != "true" && obj.Spec.DesiredNodeUnschedulable != "false" {
		return nil
	}

	node, err := nodehelper.GetNodeForMachine(obj, m.nodeLister)
	if err != nil || node == nil || node.DeletionTimestamp != nil {
		return err
	}
	desiredValue := convert.ToBool(obj.Spec.DesiredNodeUnschedulable)
	if node.Spec.Unschedulable != desiredValue {
		toUpdate := node.DeepCopy()
		toUpdate.Spec.Unschedulable = desiredValue
		if _, err := m.nodeClient.Update(toUpdate); err != nil {
			return err
		}
	}
	nodeCopy := obj.DeepCopy()
	if !desiredValue {
		removeDrainCondition(nodeCopy)
	}
	nodeCopy.Spec.DesiredNodeUnschedulable = ""
	_, err = m.machines.Update(nodeCopy)
	if err != nil {
		return err
	}
	return nil
}

func (d *NodeDrain) drainNode(key string, obj *v3.Node) error {
	if obj == nil || obj.DeletionTimestamp != nil || obj.Spec.DesiredNodeUnschedulable == "" {
		return nil
	}

	if obj.Spec.DesiredNodeUnschedulable != "drain" && obj.Spec.DesiredNodeUnschedulable != "stopDrain" {
		return nil
	}

	if obj.Spec.DesiredNodeUnschedulable == "drain" {
		nodeMapLock.Lock()
		if _, ok := d.nodesToContext[obj.Name]; ok {
			nodeMapLock.Unlock()
			return nil
		}
		ctx, cancel := context.WithCancel(d.ctx)
		d.nodesToContext[obj.Name] = cancel
		go d.drain(ctx, obj, cancel)
		nodeMapLock.Unlock()

	} else if obj.Spec.DesiredNodeUnschedulable == "stopDrain" {
		nodeMapLock.Lock()
		cancelFunc, ok := d.nodesToContext[obj.Name]
		nodeMapLock.Unlock()
		if ok {
			cancelFunc()
		}
		return d.resetDesiredNodeUnschedulable(obj)
	}
	return nil
}

func (d *NodeDrain) updateNode(node *v3.Node, updateFunc func(node *v3.Node, originalErr error, kubeErr error), originalErr error, kubeErr error) (*v3.Node, error) {
	updatedObj, err := d.machines.Update(node)
	if err != nil && errors.IsConflict(err) {
		// retrying twelve times, if conflict error still exists, give up
		for i := 0; i < 12; i++ {
			latestObj, err := d.machines.Get(node.Name, metav1.GetOptions{})
			if err != nil {
				logrus.Warnf("nodeDrain: error fetching node %s", node.Spec.RequestedHostname)
				return nil, err
			}
			updateFunc(latestObj, originalErr, kubeErr)
			updatedObj, err = d.machines.Update(latestObj)
			if err != nil && errors.IsConflict(err) {
				logrus.Debugf("nodeDrain: conflict error, will retry again %s", node.Spec.RequestedHostname)
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return updatedObj, err
		}
	}
	return updatedObj, err
}

func (d *NodeDrain) drain(ctx context.Context, obj *v3.Node, cancel context.CancelFunc) {
	defer deleteFromContextMap(d.nodesToContext, obj.Name)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		stopped := false
		nodeName := obj.Spec.RequestedHostname
		updatedObj, err := v3.NodeConditionDrained.DoUntilTrue(obj, func() (runtime.Object, error) {
			kubeConfig, err := d.getKubeConfig()
			if err != nil {
				logrus.Errorf("nodeDrain: error getting kubeConfig for node %s", obj.Name)
				return obj, fmt.Errorf("error getting kubeConfig for node %s", obj.Name)
			}
			nodeCopy := obj.DeepCopy()
			setConditionDraining(nodeCopy, nil, nil)
			nodeObj, err := d.updateNode(nodeCopy, setConditionDraining, nil, nil)
			if err != nil {
				return obj, err
			}
			logrus.Infof("Draining node %s in %s", nodeName, obj.Namespace)
			_, msg, err := kubectl.Drain(ctx, kubeConfig, nodeName, getFlags(nodeObj.Spec.NodeDrainInput))
			if err != nil {
				if ctx.Err() == context.Canceled {
					stopped = true
					logrus.Infof(fmt.Sprintf("Stopped draining %s in %s", nodeName, obj.Namespace))
					return nodeObj, nil
				}
				errMsg := filterErrorMsg(msg, nodeName)
				return nodeObj, fmt.Errorf("%s", errMsg)
			}
			return nodeObj, nil
		})
		kubeErr := err
		if err != nil {
			ignore, timeoutErr := ignoreErr(err.Error())
			if ignore {
				kubeErr = nil
				if timeoutErr {
					err = fmt.Errorf(fmt.Sprintf("Drain failed: drain did not complete within %vs",
						obj.Spec.NodeDrainInput.Timeout))
				}
			}
		}
		if !stopped {
			nodeCopy := updatedObj.(*v3.Node).DeepCopy()
			setConditionComplete(nodeCopy, err, kubeErr)
			_, updateErr := d.updateNode(nodeCopy, setConditionComplete, err, kubeErr)
			if kubeErr != nil || updateErr != nil {
				logrus.Errorf("nodeDrain: [%s] in cluster [%s] : %v %v", nodeName, d.clusterName, err, updateErr)
				d.machines.Controller().Enqueue("", fmt.Sprintf("%s/%s", d.clusterName, obj.Name))
			}
			cancel()
		}
	}
}

func (d *NodeDrain) resetDesiredNodeUnschedulable(obj *v3.Node) error {
	nodeCopy := obj.DeepCopy()
	removeDrainCondition(nodeCopy)
	nodeCopy.Spec.DesiredNodeUnschedulable = ""
	if _, err := d.machines.Update(nodeCopy); err != nil {
		return err
	}
	return nil
}

func (d *NodeDrain) getKubeConfig() (*clientcmdapi.Config, error) {
	cluster, err := d.clusterLister.Get("", d.clusterName)
	if err != nil {
		return nil, err
	}
	user, err := d.systemAccountManager.GetSystemUser(cluster)
	if err != nil {
		return nil, err
	}
	token, err := d.userManager.EnsureToken(drainTokenPrefix+user.Name, description, user.Name)
	if err != nil {
		return nil, err
	}
	kubeConfig := d.kubeConfigGetter.KubeConfig(d.clusterName, token)
	for k := range kubeConfig.Clusters {
		kubeConfig.Clusters[k].InsecureSkipTLSVerify = true
	}
	return kubeConfig, nil
}

func getFlags(input *v3.NodeDrainInput) []string {
	return []string{
		fmt.Sprintf("--delete-local-data=%v", input.DeleteLocalData),
		fmt.Sprintf("--force=%v", input.Force),
		fmt.Sprintf("--grace-period=%v", input.GracePeriod),
		fmt.Sprintf("--ignore-daemonsets=%v", input.IgnoreDaemonSets),
		fmt.Sprintf("--timeout=%s", convert.ToString(input.Timeout)+"s")}
}

func filterErrorMsg(msg string, nodeName string) string {
	upd := []string{}
	lines := strings.Split(msg, "\n")
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "WARNING") || strings.HasPrefix(line, nodeName) {
			continue
		}
		if strings.Contains(line, "aborting") {
			continue
		}
		if strings.HasPrefix(line, "There are pending nodes ") {
			// for only one node in our case
			continue
		}
		if strings.HasPrefix(line, "There are pending pods ") {
			// already considered error
			continue
		}
		if strings.HasPrefix(line, "error") && strings.Contains(line, "unable to drain node") {
			// actual reason at end
			continue
		}
		if strings.HasPrefix(line, "pod") && strings.Contains(line, "evicted") {
			// evicted successfully
			continue
		}
		upd = append(upd, line)
	}
	return strings.Join(upd, "\n")
}

func removeDrainCondition(obj *v3.Node) {
	exists := false
	for _, condition := range obj.Status.Conditions {
		if condition.Type == "Drained" {
			exists = true
			break
		}
	}
	if exists {
		var conditions []v3.NodeCondition
		for _, condition := range obj.Status.Conditions {
			if condition.Type == "Drained" {
				continue
			}
			conditions = append(conditions, condition)
		}
		obj.Status.Conditions = conditions
	}
}

func deleteFromContextMap(data map[string]context.CancelFunc, id string) {
	nodeMapLock.Lock()
	delete(data, id)
	nodeMapLock.Unlock()
}

func ignoreErr(msg string) (bool, bool) {
	for _, val := range toIgnoreErrs {
		if strings.Contains(msg, val) {
			// check if timeout error
			if !strings.HasPrefix(val, "--") {
				return true, true
			}
			return true, false
		}
	}
	return false, false
}

func setConditionDraining(node *v3.Node, err error, kubeErr error) {
	v3.NodeConditionDrained.Unknown(node)
	v3.NodeConditionDrained.Reason(node, "")
	v3.NodeConditionDrained.Message(node, "")
}

func setConditionComplete(node *v3.Node, err error, kubeErr error) {
	if err == nil {
		v3.NodeConditionDrained.True(node)
	} else {
		v3.NodeConditionDrained.False(node)
		v3.NodeConditionDrained.ReasonAndMessageFromError(node, err)
	}
	if kubeErr == nil {
		node.Spec.DesiredNodeUnschedulable = ""
	}
}
