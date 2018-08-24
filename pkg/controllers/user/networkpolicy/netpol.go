package networkpolicy

import (
	"fmt"
	"net"
	"reflect"
	"sort"

	"github.com/rancher/rancher/pkg/controllers/user/nslabels"
	typescorev1 "github.com/rancher/types/apis/core/v1"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	rnetworkingv1 "github.com/rancher/types/apis/networking.k8s.io/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	knetworkingv1 "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	//FlannelPresenceLabel is used to detect if a node is using flannel plugin or not
	FlannelPresenceLabel = "flannel.alpha.coreos.com/public-ip"
	systemProjectLabel   = "authz.management.cattle.io/system-project"
)

type netpolMgr struct {
	nsLister         typescorev1.NamespaceLister
	nodeLister       typescorev1.NodeLister
	pods             typescorev1.PodInterface
	projects         v3.ProjectInterface
	npLister         rnetworkingv1.NetworkPolicyLister
	npClient         rnetworkingv1.Interface
	projLister       v3.ProjectLister
	clusterNamespace string
}

func (npmgr *netpolMgr) program(np *knetworkingv1.NetworkPolicy) error {
	existing, err := npmgr.npLister.Get(np.Namespace, np.Name)
	logrus.Debugf("netpolMgr: program: existing=%+v, err=%v", existing, err)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logrus.Debugf("netpolMgr: program: about to create np=%+v", *np)
			_, err = npmgr.npClient.NetworkPolicies(np.Namespace).Create(np)
			if err != nil && !kerrors.IsAlreadyExists(err) && !kerrors.IsForbidden(err) {
				return fmt.Errorf("netpolMgr: program: error creating network policy err=%v", err)
			}
		} else {
			return fmt.Errorf("netpolMgr: program: got unexpected error while getting network policy=%v", err)
		}
	} else {
		logrus.Debugf("netpolMgr: program: existing=%+v", existing)
		if existing.DeletionTimestamp == nil && !reflect.DeepEqual(existing.Spec, np.Spec) {
			logrus.Debugf("netpolMgr: program: about to update np=%+v", *np)
			_, err = npmgr.npClient.NetworkPolicies(np.Namespace).Update(np)
			if err != nil {
				return fmt.Errorf("netpolMgr: program: error updating network policy err=%v", err)
			}
		} else {
			logrus.Debugf("netpolMgr: program: no need to update np=%+v", *np)
		}
	}
	return nil
}

func (npmgr *netpolMgr) delete(policyNamespace, policyName string) error {
	existing, err := npmgr.npLister.Get(policyNamespace, policyName)
	logrus.Debugf("netpolMgr: delete: existing=%+v, err=%v", existing, err)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("netpolMgr: delete: got unexpected error while getting network policy=%v", err)
	}
	logrus.Debugf("netpolMgr: delete: existing=%+v", existing)
	err = npmgr.npClient.NetworkPolicies(existing.Namespace).Delete(existing.Name, &v1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("netpolMgr: delete: error deleting network policy err=%v", err)
	}
	return nil
}

func (npmgr *netpolMgr) programNetworkPolicy(projectID string, clusterNamespace string) error {
	logrus.Debugf("netpolMgr: programNetworkPolicy: projectID=%v", projectID)
	// Get namespaces belonging to project
	set := labels.Set(map[string]string{nslabels.ProjectIDFieldLabel: projectID})
	namespaces, err := npmgr.nsLister.List("", set.AsSelector())
	if err != nil {
		return fmt.Errorf("netpolMgr: couldn't list namespaces with projectID %v err=%v", projectID, err)
	}
	logrus.Debugf("netpolMgr: programNetworkPolicy: namespaces=%+v", namespaces)

	systemNamespaces, systemProjectID, err := npmgr.getSystemNSInfo(clusterNamespace)
	if err != nil {
		return fmt.Errorf("netpolMgr: programNetworkPolicy getSystemNamespaces: err=%v", err)
	}
	policyName := "np-default"
	for _, aNS := range namespaces {
		id, _ := aNS.Labels[nslabels.ProjectIDFieldLabel]
		if systemNamespaces[aNS.Name] || id == "" {
			npmgr.delete(aNS.Name, policyName)
			continue
		}
		if aNS.DeletionTimestamp != nil {
			logrus.Debugf("netpolMgr: programNetworkPolicy: aNS=%+v marked for deletion, skipping", aNS)
			continue
		}
		np := generateDefaultNamespaceNetworkPolicy(aNS, projectID, systemProjectID)
		if err := npmgr.program(np); err != nil {
			return fmt.Errorf("netpolMgr: programNetworkPolicy: error programming default network policy for ns=%v err=%v", aNS.Name, err)
		}
	}
	return nil
}

func (npmgr *netpolMgr) handleHostNetwork(clusterNamespace string) error {
	nodes, err := npmgr.nodeLister.List("", labels.Everything())
	if err != nil {
		return fmt.Errorf("couldn't list nodes err=%v", err)
	}

	logrus.Debugf("netpolMgr: handleHostNetwork: processing %d nodes", len(nodes))
	policyName := "hn-nodes"
	np := generateNodesNetworkPolicy()
	for _, node := range nodes {
		if _, ok := node.Annotations[FlannelPresenceLabel]; !ok {
			logrus.Debugf("netpolMgr: handleHostNetwork: node=%v doesn't have flannel label, skipping", node.Name)
			continue
		}
		podCIDRFirstIP, _, err := net.ParseCIDR(node.Spec.PodCIDR)
		if err != nil {
			logrus.Debugf("netpolMgr: handleHostNetwork: node=%+v", node)
			logrus.Errorf("netpolMgr: handleHostNetwork: couldn't parse PodCIDR(%v) for node %v err=%v", node.Spec.PodCIDR, node.Name, err)
			continue
		}
		ipBlock := knetworkingv1.IPBlock{
			CIDR: podCIDRFirstIP.String() + "/32",
		}
		np.Spec.Ingress[0].From = append(np.Spec.Ingress[0].From, knetworkingv1.NetworkPolicyPeer{IPBlock: &ipBlock})
	}

	// sort ipblocks so it always appears in a certain order
	sort.Slice(np.Spec.Ingress[0].From, func(i, j int) bool {
		return np.Spec.Ingress[0].From[i].IPBlock.CIDR < np.Spec.Ingress[0].From[j].IPBlock.CIDR
	})

	namespaces, err := npmgr.nsLister.List("", labels.Everything())
	if err != nil {
		return fmt.Errorf("couldn't list namespaces err=%v", err)
	}

	systemNamespaces, _, err := npmgr.getSystemNSInfo(clusterNamespace)
	if err != nil {
		return fmt.Errorf("netpolMgr: handleHostNetwork getSystemNamespaces: err=%v", err)
	}
	for _, aNS := range namespaces {
		projectID, _ := aNS.Labels[nslabels.ProjectIDFieldLabel]
		if systemNamespaces[aNS.Name] || projectID == "" {
			npmgr.delete(aNS.Name, policyName)
			continue
		}
		if aNS.DeletionTimestamp != nil || aNS.Status.Phase == corev1.NamespaceTerminating {
			logrus.Debugf("netpolMgr: handleHostNetwork: aNS=%+v marked for deletion/termination, skipping", aNS)
			continue
		}
		if _, ok := aNS.Labels[nslabels.ProjectIDFieldLabel]; !ok {
			continue
		}

		logrus.Debugf("netpolMgr: handleHostNetwork: aNS=%+v", aNS)

		np.OwnerReferences = []v1.OwnerReference{
			{
				APIVersion: "v1",
				Kind:       "Namespace",
				UID:        aNS.UID,
				Name:       aNS.Name,
			},
		}
		np.Namespace = aNS.Name
		if err := npmgr.program(np); err != nil {
			logrus.Errorf("netpolMgr: handleHostNetwork: error programming hostNetwork network policy for ns=%v err=%v", aNS.Name, err)
		}
	}
	return nil
}

func (npmgr *netpolMgr) getSystemNSInfo(clusterNamespace string) (map[string]bool, string, error) {
	systemNamespaces := map[string]bool{}
	set := labels.Set(map[string]string{systemProjectLabel: "true"})
	projects, err := npmgr.projLister.List(clusterNamespace, set.AsSelector())
	systemProjectID := ""
	if err != nil {
		return nil, systemProjectID, err
	}
	if len(projects) == 0 {
		return systemNamespaces, systemProjectID,
			fmt.Errorf("systemNamespaces: no system project for cluster %s", clusterNamespace)
	}
	if len(projects) > 1 {
		return systemNamespaces, systemProjectID,
			fmt.Errorf("systemNamespaces: more than one system project in cluster %s", clusterNamespace)
	}
	// ns.Annotations[projectIDAnnotation] = fmt.Sprintf("%v:%v", n.m.clusterName, projects[0].Name)
	// ns.Labels[ProjectIDFieldLabel] = projectID / projects[0].Name
	systemProjectID = projects[0].Name
	if systemProjectID == "" {
		return nil, systemProjectID, fmt.Errorf("sytemNamespaces: system project id cannot be empty")
	}
	set = labels.Set(map[string]string{nslabels.ProjectIDFieldLabel: systemProjectID})
	namespaces, err := npmgr.nsLister.List("", set.AsSelector())
	if err != nil {
		return nil, systemProjectID,
			fmt.Errorf("sytemNamespaces: couldn't list namespaces err=%v", err)
	}
	for _, ns := range namespaces {
		if _, ok := systemNamespaces[ns.Name]; !ok {
			systemNamespaces[ns.Name] = true
		}
	}
	return systemNamespaces, systemProjectID, nil
}

func generateDefaultNamespaceNetworkPolicy(aNS *corev1.Namespace, projectID string, systemProjectID string) *knetworkingv1.NetworkPolicy {
	policyName := "np-default"
	np := &knetworkingv1.NetworkPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: aNS.Name,
			Labels:    labels.Set(map[string]string{nslabels.ProjectIDFieldLabel: projectID}),
		},
		Spec: knetworkingv1.NetworkPolicySpec{
			// An empty PodSelector selects all pods in this Namespace.
			PodSelector: v1.LabelSelector{},
			Ingress: []knetworkingv1.NetworkPolicyIngressRule{
				{
					From: []knetworkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &v1.LabelSelector{
								MatchLabels: map[string]string{nslabels.ProjectIDFieldLabel: projectID},
							},
						},
						{
							NamespaceSelector: &v1.LabelSelector{
								MatchLabels: map[string]string{nslabels.ProjectIDFieldLabel: systemProjectID},
							},
						},
					},
				},
			},
			PolicyTypes: []knetworkingv1.PolicyType{
				knetworkingv1.PolicyTypeIngress,
			},
		},
	}
	return np
}

func generateNodesNetworkPolicy() *knetworkingv1.NetworkPolicy {
	policyName := "hn-nodes"
	np := &knetworkingv1.NetworkPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name: policyName,
		},
		Spec: knetworkingv1.NetworkPolicySpec{
			PodSelector: v1.LabelSelector{},
			Ingress: []knetworkingv1.NetworkPolicyIngressRule{
				{
					From: []knetworkingv1.NetworkPolicyPeer{},
				},
			},
			PolicyTypes: []knetworkingv1.PolicyType{
				knetworkingv1.PolicyTypeIngress,
			},
		},
	}
	return np
}

func portToString(port knetworkingv1.NetworkPolicyPort) string {
	return fmt.Sprintf("%v/%v", port.Port, port.Protocol)
}
