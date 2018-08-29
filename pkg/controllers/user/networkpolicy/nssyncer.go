package networkpolicy

import (
	"fmt"

	"github.com/rancher/rancher/pkg/controllers/user/nslabels"
	"github.com/rancher/types/apis/core/v1"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type nsSyncer struct {
	npmgr            *netpolMgr
	clusterLister    v3.ClusterLister
	serviceLister    v1.ServiceLister
	podLister        v1.PodLister
	serviceClient    v1.ServiceInterface
	podClient        v1.PodInterface
	clusterNamespace string
}

// Sync invokes Policy Handler to program the native network policies
func (nss *nsSyncer) Sync(key string, ns *corev1.Namespace) error {
	if ns == nil || ns.DeletionTimestamp != nil {
		return nil
	}

	disabled, err := isNetworkPolicyDisabled(nss.clusterNamespace, nss.clusterLister)
	if err != nil {
		return err
	}
	if disabled {
		return nil
	}

	logrus.Debugf("nsSyncer: Sync: %v, %+v", ns.Name, *ns)

	projectID := ns.Labels[nslabels.ProjectIDFieldLabel]
	movedToNone := projectID == ""
	if !movedToNone {
		logrus.Debugf("nsSyncer: Sync: ns=%v projectID=%v", ns.Name, projectID)
		// program project isolation network policy
		if err := nss.npmgr.programNetworkPolicy(projectID, nss.clusterNamespace); err != nil {
			return fmt.Errorf("nsSyncer: Sync: error programming network policy: %v (ns=%v, projectID=%v), ", err, ns.Name, projectID)
		}
	}
	// handle moving of namespace between projects
	if err := nss.syncOnMove(ns.Name, projectID, movedToNone); err != nil {
		return fmt.Errorf("nsSyncer: Sync: error moving network policy: %v (ns=%v, projectID=%v), ", err, ns.Name, projectID)
	}
	if movedToNone {
		return nil
	}

	// handle if hostNetwork policy is needed
	return nss.npmgr.handleHostNetwork(nss.clusterNamespace)
}

func (nss *nsSyncer) syncOnMove(nsName string, projectID string, movedToNone bool) error {
	systemNamespaces, _, err := nss.npmgr.getSystemNSInfo(nss.clusterNamespace)
	if err != nil {
		return fmt.Errorf("nsSyncer: error getting systemNamespaces %v", err)
	}
	if movedToNone {
		nss.npmgr.delete(nsName, "np-default")
		nss.npmgr.delete(nsName, "hn-nodes")
	}
	if err = nss.syncNodePortServices(systemNamespaces, nsName, movedToNone); err != nil {
		return fmt.Errorf("nsSyncer: error syncing services %v", err)
	}
	if err = nss.syncHostPortPods(systemNamespaces, nsName, movedToNone); err != nil {
		return fmt.Errorf("nsSyncer: error syncing pods %v", err)
	}
	return nil
}

func (nss *nsSyncer) syncNodePortServices(systemNamespaces map[string]bool, nsName string, moved bool) error {
	svcs, err := nss.serviceLister.List(nsName, labels.NewSelector())
	if err != nil {
		return err
	}
	for _, svc := range svcs {
		if systemNamespaces[svc.Namespace] || moved {
			policyName := getNodePortsPolicyName(svc)
			nss.npmgr.delete(svc.Namespace, policyName)
			continue
		}
		if nodePortService(svc) {
			nss.serviceClient.Controller().Enqueue(svc.Namespace, svc.Name)
		}
	}
	return nil
}

func (nss *nsSyncer) syncHostPortPods(systemNamespaces map[string]bool, nsName string, moved bool) error {
	pods, err := nss.podLister.List(nsName, labels.NewSelector())
	if err != nil {
		return err
	}
	for _, pod := range pods {
		if systemNamespaces[pod.Namespace] || moved {
			policyName := getHostPortsPolicyName(pod)
			nss.npmgr.delete(pod.Namespace, policyName)
			continue
		}
		if hostPortPod(pod) {
			nss.podClient.Controller().Enqueue(pod.Namespace, pod.Name)
		}
	}
	return nil
}
