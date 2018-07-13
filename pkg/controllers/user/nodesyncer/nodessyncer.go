package nodesyncer

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	nodehelper "github.com/rancher/rancher/pkg/node"
	"github.com/rancher/types/apis/core/v1"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	AllNodeKey     = "_machine_all_"
	annotationName = "management.cattle.io/nodesyncer"
)

type NodeSyncer struct {
	machines         v3.NodeInterface
	clusterNamespace string
	nodesSyncer      *NodesSyncer
}

type PodsStatsSyncer struct {
	clusterName      string
	clusterNamespace string
	machinesClient   v3.NodeInterface
}

type NodesSyncer struct {
	machines         v3.NodeInterface
	machineLister    v3.NodeLister
	nodeLister       v1.NodeLister
	nodeClient       v1.NodeInterface
	podLister        v1.PodLister
	clusterNamespace string
	clusterName      string
	clusterLister    v3.ClusterLister
}

func Register(cluster *config.UserContext) {
	m := &NodesSyncer{
		clusterNamespace: cluster.ClusterName,
		machines:         cluster.Management.Management.Nodes(cluster.ClusterName),
		machineLister:    cluster.Management.Management.Nodes(cluster.ClusterName).Controller().Lister(),
		nodeLister:       cluster.Core.Nodes("").Controller().Lister(),
		nodeClient:       cluster.Core.Nodes(""),
		podLister:        cluster.Core.Pods("").Controller().Lister(),
		clusterName:      cluster.ClusterName,
		clusterLister:    cluster.Management.Management.Clusters("").Controller().Lister(),
	}

	n := &NodeSyncer{
		clusterNamespace: cluster.ClusterName,
		machines:         cluster.Management.Management.Nodes(cluster.ClusterName),
		nodesSyncer:      m,
	}

	cluster.Core.Nodes("").Controller().AddHandler("nodesSyncer", n.sync)
	cluster.Management.Management.Nodes(cluster.ClusterName).Controller().AddHandler("machinesSyncer", m.sync)
	cluster.Management.Management.Nodes(cluster.ClusterName).Controller().AddHandler("machinesLabelSyncer", m.syncLabels)
	cluster.Management.Management.Nodes(cluster.ClusterName).Controller().AddHandler("cordonFieldsSyncer", m.syncCordonFields)
}

func (n *NodeSyncer) sync(key string, node *corev1.Node) error {
	needUpdate, err := n.needUpdate(key, node)
	if err != nil {
		return err
	}
	if needUpdate {
		n.machines.Controller().Enqueue(n.clusterNamespace, AllNodeKey)
	}

	return nil
}

func (n *NodeSyncer) needUpdate(key string, node *corev1.Node) (bool, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return true, nil
	}
	existing, err := n.nodesSyncer.getNodeForNode(node, true)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return true, nil
	}
	nodeToPodMap, err := n.nodesSyncer.getNonTerminatedPods()
	if err != nil {
		return false, err
	}
	toUpdate, err := n.nodesSyncer.convertNodeToNode(node, existing, nodeToPodMap)
	if err != nil {
		return false, err
	}

	// update only when nothing changed
	if objectsAreEqual(existing, toUpdate) {
		return false, nil
	}
	return true, nil
}

func (m *NodesSyncer) sync(key string, machine *v3.Node) error {
	if key == fmt.Sprintf("%s/%s", m.clusterNamespace, AllNodeKey) {
		return m.reconcileAll()
	}
	return nil
}

func (m *NodesSyncer) getNode(machine *v3.Node, nodes []*corev1.Node) (*corev1.Node, error) {
	for _, n := range nodes {
		if nodehelper.IsNodeForNode(n, machine) {
			return n, nil
		}
	}

	return nil, nil
}

func (m *NodesSyncer) syncLabels(key string, obj *v3.Node) error {
	if obj == nil {
		return nil
	}

	if obj.Spec.DesiredNodeAnnotations == nil && obj.Spec.DesiredNodeLabels == nil {
		return nil
	}

	nodes, err := m.nodeLister.List("", labels.NewSelector())
	if err != nil {
		return err
	}
	node, err := m.getNode(obj, nodes)
	if err != nil || node == nil {
		return err
	}

	updateLabels := false
	updateAnnotations := false
	// set annotations
	if obj.Spec.DesiredNodeAnnotations != nil && !reflect.DeepEqual(node.Annotations, obj.Spec.DesiredNodeAnnotations) {
		updateAnnotations = true
	}
	// set labels
	if obj.Spec.DesiredNodeLabels != nil && !reflect.DeepEqual(node.Labels, obj.Spec.DesiredNodeLabels) {
		updateLabels = true
	}

	if updateLabels || updateAnnotations {
		toUpdate := node.DeepCopy()
		if updateLabels {
			toUpdate.Labels = obj.Spec.DesiredNodeLabels
		}
		if updateAnnotations {
			toUpdate.Annotations = obj.Spec.DesiredNodeAnnotations
		}
		logrus.Infof("Updating node %v with labels %v and annotations %v", toUpdate.Name, toUpdate.Labels, toUpdate.Annotations)
		if _, err := m.nodeClient.Update(toUpdate); err != nil {
			return err
		}
	}

	// in the end we reset all desired fields
	if obj.Spec.DesiredNodeAnnotations != nil || obj.Spec.DesiredNodeLabels != nil {
		machine := obj.DeepCopy()
		machine.Spec.DesiredNodeAnnotations = nil
		machine.Spec.DesiredNodeLabels = nil
		if _, err := m.machines.Update(machine); err != nil {
			return err
		}
	}

	return nil
}

func (m *NodesSyncer) reconcileAll() error {
	nodes, err := m.nodeLister.List("", labels.NewSelector())
	if err != nil {
		return err
	}

	nodeMap := make(map[string]*corev1.Node)
	for _, node := range nodes {
		nodeMap[node.Name] = node
	}

	machines, err := m.machineLister.List(m.clusterNamespace, labels.NewSelector())
	machineMap := make(map[string]*v3.Node)
	toDelete := make(map[string]*v3.Node)
	for _, machine := range machines {
		node, err := m.getNode(machine, nodes)
		if err != nil {
			return err
		}
		if node == nil {
			logrus.Debugf("Failed to get node for machine [%s]", machine.Name)
			toDelete[machine.Name] = machine
			continue
		}
		machineMap[node.Name] = machine
	}
	nodeToPodMap, err := m.getNonTerminatedPods()
	if err != nil {
		return err
	}

	// reconcile machines for existing nodes
	for name, node := range nodeMap {
		machine, _ := machineMap[name]
		err = m.reconcileNodeForNode(machine, node, nodeToPodMap)
		if err != nil {
			return err
		}
	}
	// run the logic for machine to remove
	for name, machine := range machineMap {
		if _, ok := nodeMap[name]; !ok {
			if err := m.removeNode(machine); err != nil {
				return err
			}
		}
	}

	for _, machine := range toDelete {
		if err := m.removeNode(machine); err != nil {
			return err
		}
	}

	return nil
}

func (m *NodesSyncer) reconcileNodeForNode(machine *v3.Node, node *corev1.Node, pods map[string][]*corev1.Pod) error {
	if machine == nil {
		return m.createNode(node, pods)
	}
	return m.updateNode(machine, node, pods)
}

func (m *NodesSyncer) removeNode(machine *v3.Node) error {
	if machine.Annotations == nil {
		return nil
	}

	if _, ok := machine.Annotations[annotationName]; !ok {
		return nil
	}

	err := m.machines.Delete(machine.ObjectMeta.Name, nil)
	if err != nil {
		return errors.Wrapf(err, "Failed to delete machine [%s]", machine.Name)
	}
	logrus.Infof("Deleted cluster node [%s]", machine.Name)
	return nil
}

func (m *NodesSyncer) updateNode(existing *v3.Node, node *corev1.Node, pods map[string][]*corev1.Pod) error {
	toUpdate, err := m.convertNodeToNode(node, existing, pods)
	if err != nil {
		return err
	}
	// update only when nothing changed
	if objectsAreEqual(existing, toUpdate) {
		return nil
	}
	logrus.Debugf("Updating machine for node [%s]", node.Name)
	_, err = m.machines.Update(toUpdate)
	if err != nil {
		return errors.Wrapf(err, "Failed to update machine for node [%s]", node.Name)
	}
	logrus.Debugf("Updated machine for node [%s]", node.Name)
	return nil
}

func (m *NodesSyncer) createNode(node *corev1.Node, pods map[string][]*corev1.Pod) error {
	// do not create machine for rke cluster
	cluster, err := m.clusterLister.Get("", m.clusterName)
	if err != nil {
		return err
	}
	if cluster.Spec.RancherKubernetesEngineConfig != nil {
		return nil
	}
	// try to get machine from api, in case cache didn't get the update
	existing, err := m.getNodeForNode(node, false)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	machine, err := m.convertNodeToNode(node, existing, pods)
	if err != nil {
		return err
	}

	if machine.Annotations == nil {
		machine.Annotations = make(map[string]string)
		machine.Annotations[annotationName] = "true"
	}

	_, err = m.machines.Create(machine)
	if err != nil {
		return errors.Wrapf(err, "Failed to create machine for node [%s]", node.Name)
	}
	logrus.Infof("Created machine for node [%s]", node.Name)
	return nil
}

func (m *NodesSyncer) getNodeForNode(node *corev1.Node, cache bool) (*v3.Node, error) {
	if cache {
		machines, err := m.machineLister.List(m.clusterNamespace, labels.NewSelector())
		if err != nil {
			return nil, err
		}
		for _, machine := range machines {
			if nodehelper.IsNodeForNode(node, machine) {
				return machine, nil
			}
		}
	} else {
		machines, err := m.machines.List(metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, machine := range machines.Items {
			if machine.Namespace == m.clusterNamespace {
				if nodehelper.IsNodeForNode(node, &machine) {
					return &machine, nil
				}
			}
		}
	}

	return nil, nil
}

func resetConditions(machine *v3.Node) *v3.Node {
	if machine.Status.InternalNodeStatus.Conditions == nil {
		return machine
	}
	updated := machine.DeepCopy()
	var toUpdateConds []corev1.NodeCondition
	for _, cond := range machine.Status.InternalNodeStatus.Conditions {
		toUpdateCond := cond.DeepCopy()
		toUpdateCond.LastHeartbeatTime = metav1.Time{}
		toUpdateCond.LastTransitionTime = metav1.Time{}
		toUpdateConds = append(toUpdateConds, *toUpdateCond)
	}
	updated.Status.InternalNodeStatus.Conditions = toUpdateConds
	return updated
}

func objectsAreEqual(existing *v3.Node, toUpdate *v3.Node) bool {
	// we are updating spec and status only, so compare them
	toUpdateToCompare := resetConditions(toUpdate)
	existingToCompare := resetConditions(existing)
	statusEqual := reflect.DeepEqual(toUpdateToCompare.Status.InternalNodeStatus, existingToCompare.Status.InternalNodeStatus)
	labelsEqual := reflect.DeepEqual(toUpdateToCompare.Status.NodeLabels, existing.Status.NodeLabels)
	annotationsEqual := reflect.DeepEqual(toUpdateToCompare.Status.NodeAnnotations, existing.Status.NodeAnnotations)
	specEqual := reflect.DeepEqual(toUpdateToCompare.Spec.InternalNodeSpec, existingToCompare.Spec.InternalNodeSpec)
	nodeNameEqual := toUpdateToCompare.Status.NodeName == existingToCompare.Status.NodeName
	requestsEqual := isEqual(toUpdateToCompare.Status.Requested, existingToCompare.Status.Requested)
	limitsEqual := isEqual(toUpdateToCompare.Status.Limits, existingToCompare.Status.Limits)
	rolesEqual := toUpdateToCompare.Spec.Worker == existingToCompare.Spec.Worker && toUpdateToCompare.Spec.Etcd == existingToCompare.Spec.Etcd &&
		toUpdateToCompare.Spec.ControlPlane == existingToCompare.Spec.ControlPlane

	retVal := statusEqual && specEqual && nodeNameEqual && labelsEqual && annotationsEqual && requestsEqual && limitsEqual && rolesEqual
	if !retVal {
		logrus.Debugf("ObjectsAreEqualResults for %s: statusEqual: %t specEqual: %t"+
			" nodeNameEqual: %t labelsEqual: %t annotationsEqual: %t requestsEqual: %t limitsEqual: %t rolesEqual: %t",
			toUpdate.Name, statusEqual, specEqual, nodeNameEqual, labelsEqual, annotationsEqual, requestsEqual, limitsEqual, rolesEqual)
	}
	return retVal
}

func (m *NodesSyncer) convertNodeToNode(node *corev1.Node, existing *v3.Node, pods map[string][]*corev1.Pod) (*v3.Node, error) {
	var machine *v3.Node
	if existing == nil {
		machine = &v3.Node{
			Spec:   v3.NodeSpec{},
			Status: v3.NodeStatus{},
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "machine-"},
		}
		machine.Namespace = m.clusterNamespace
		machine.Status.Requested = make(map[corev1.ResourceName]resource.Quantity)
		machine.Status.Limits = make(map[corev1.ResourceName]resource.Quantity)
		machine.Spec.InternalNodeSpec = *node.Spec.DeepCopy()
		machine.Status.InternalNodeStatus = *node.Status.DeepCopy()
		machine.Spec.RequestedHostname = node.Name
	} else {
		machine = existing.DeepCopy()
		machine.Spec.InternalNodeSpec = *node.Spec.DeepCopy()
		machine.Status.InternalNodeStatus = *node.Status.DeepCopy()
	}

	requests, limits := aggregateRequestAndLimitsForNode(pods[node.Name])
	if machine.Status.Requested == nil {
		machine.Status.Requested = corev1.ResourceList{}
	}
	if machine.Status.Limits == nil {
		machine.Status.Limits = corev1.ResourceList{}
	}

	for name, quantity := range requests {
		machine.Status.Requested[name] = quantity
	}
	for name, quantity := range limits {
		machine.Status.Limits[name] = quantity
	}

	if node.Labels != nil {
		_, etcd := node.Labels["node-role.kubernetes.io/etcd"]
		machine.Spec.Etcd = etcd
		_, control := node.Labels["node-role.kubernetes.io/controlplane"]
		_, master := node.Labels["node-role.kubernetes.io/master"]
		if control || master {
			machine.Spec.ControlPlane = true
		}
		_, worker := node.Labels["node-role.kubernetes.io/worker"]
		machine.Spec.Worker = worker
	}

	machine.Status.NodeAnnotations = node.Annotations
	machine.Status.NodeLabels = node.Labels
	machine.Status.NodeName = node.Name
	machine.APIVersion = "management.cattle.io/v3"
	machine.Kind = "Node"
	v3.NodeConditionRegistered.True(machine)
	v3.NodeConditionRegistered.Message(machine, "registered with kubernetes")
	return machine, nil
}

func (m *NodesSyncer) getNonTerminatedPods() (map[string][]*corev1.Pod, error) {
	pods := make(map[string][]*corev1.Pod)
	fromCache, err := m.podLister.List("", labels.NewSelector())
	if err != nil {
		return pods, err
	}

	for _, pod := range fromCache {
		if pod.Spec.NodeName == "" || pod.DeletionTimestamp != nil {
			continue
		}
		// kubectl uses this cache to filter out the pods
		if pod.Status.Phase == "Succeeded" || pod.Status.Phase == "Failed" {
			continue
		}
		var nodePods []*corev1.Pod
		if fromMap, ok := pods[pod.Spec.NodeName]; ok {
			nodePods = fromMap
		}
		nodePods = append(nodePods, pod)
		pods[pod.Spec.NodeName] = nodePods
	}
	return pods, nil
}

func aggregateRequestAndLimitsForNode(pods []*corev1.Pod) (map[corev1.ResourceName]resource.Quantity, map[corev1.ResourceName]resource.Quantity) {
	requests, limits := map[corev1.ResourceName]resource.Quantity{}, map[corev1.ResourceName]resource.Quantity{}
	podsData := make(map[string]map[string]map[corev1.ResourceName]resource.Quantity)
	if pods != nil {
		//podName -> req/limit -> data
		for _, pod := range pods {
			podsData[pod.Name] = make(map[string]map[corev1.ResourceName]resource.Quantity)
			requests, limits := getPodData(pod)
			podsData[pod.Name]["requests"] = requests
			podsData[pod.Name]["limits"] = limits
		}
		requests[corev1.ResourcePods] = *resource.NewQuantity(int64(len(pods)), resource.DecimalSI)
	}
	for _, podData := range podsData {
		podRequests, podLimits := podData["requests"], podData["limits"]
		addMap(podRequests, requests)
		addMap(podLimits, limits)
	}
	return requests, limits
}

func isEqual(data1 map[corev1.ResourceName]resource.Quantity, data2 map[corev1.ResourceName]resource.Quantity) bool {
	if (data1 == nil || len(data1) == 0) && (data2 == nil || len(data2) == 0) {
		return true
	}
	if data1 == nil || data2 == nil {
		return false
	}
	for key, value := range data1 {
		if _, exists := data2[key]; !exists {
			return false
		}
		value2 := data2[key]
		if value.Value() != value2.Value() {
			return false
		}
	}
	return true
}

func getPodData(pod *corev1.Pod) (map[corev1.ResourceName]resource.Quantity, map[corev1.ResourceName]resource.Quantity) {
	requests, limits := map[corev1.ResourceName]resource.Quantity{}, map[corev1.ResourceName]resource.Quantity{}
	for _, container := range pod.Spec.Containers {
		addMap(container.Resources.Requests, requests)
		addMap(container.Resources.Limits, limits)
	}

	for _, container := range pod.Spec.InitContainers {
		addMapForInit(container.Resources.Requests, requests)
		addMapForInit(container.Resources.Limits, limits)
	}
	return requests, limits
}

func addMap(data1 map[corev1.ResourceName]resource.Quantity, data2 map[corev1.ResourceName]resource.Quantity) {
	for name, quantity := range data1 {
		if value, ok := data2[name]; !ok {
			data2[name] = *quantity.Copy()
		} else {
			value.Add(quantity)
			data2[name] = value
		}
	}
}

func addMapForInit(data1 map[corev1.ResourceName]resource.Quantity, data2 map[corev1.ResourceName]resource.Quantity) {
	for name, quantity := range data1 {
		value, ok := data2[name]
		if !ok {
			data2[name] = *quantity.Copy()
			continue
		}
		if quantity.Cmp(value) > 0 {
			data2[name] = *quantity.Copy()
		}
	}
}
