package workloadservice

import (
	"context"

	"fmt"

	"strings"

	"sync"

	"github.com/pkg/errors"
	"github.com/rancher/types/apis/apps/v1beta2"
	"github.com/rancher/types/apis/core/v1"
	"github.com/rancher/types/config"
	"github.com/rancher/workload-controller/controller/dnsrecord"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// This controller is responsible for monitoring services with targetWorkloadIds,
// locating corresponding pods, and marking them with the label to satisfy service selector

const (
	WorkloadAnnotation    = "field.cattle.io/targetWorkloadIds"
	WorkloadIDLabelPrefix = "workloadID"
)

var WorkloadServiceUUIDToDeploymentUUIDs sync.Map

type Controller struct {
	pods             v1.PodInterface
	deploymentLister v1beta2.DeploymentLister
	podLister        v1.PodLister
	namespaceLister  v1.NamespaceLister
	services         v1.ServiceInterface
}

func Register(ctx context.Context, workload *config.WorkloadContext) {
	c := &Controller{
		pods:             workload.Core.Pods(""),
		deploymentLister: workload.Apps.Deployments("").Controller().Lister(),
		podLister:        workload.Core.Pods("").Controller().Lister(),
		namespaceLister:  workload.Core.Namespaces("").Controller().Lister(),
		services:         workload.Core.Services(""),
	}
	workload.Core.Services("").AddHandler(c.GetName(), c.sync)
}

func (c *Controller) GetName() string {
	return "workloadServiceController"
}

func (c *Controller) sync(key string, obj *corev1.Service) error {
	if obj == nil {
		// delete from the workload map
		WorkloadServiceUUIDToDeploymentUUIDs.Delete(key)
		return nil
	}

	return c.reconcilePods(key, obj)
}

func (c *Controller) reconcilePods(key string, obj *corev1.Service) error {
	if obj.Annotations == nil {
		return nil
	}
	value, ok := obj.Annotations[WorkloadAnnotation]
	if !ok {
		return nil
	}
	workdloadIDs := strings.Split(value, ",")

	if obj.Spec.Selector == nil {
		obj.Spec.Selector = make(map[string]string)
	}
	selectorToAdd := getServiceSelector(obj)
	var toUpdate *corev1.Service
	if _, ok := obj.Spec.Selector[selectorToAdd]; !ok {
		toUpdate = obj.DeepCopy()
		toUpdate.Spec.Selector[selectorToAdd] = "true"
	}
	if err := c.updatePods(key, obj, workdloadIDs); err != nil {
		return err
	}
	if toUpdate != nil {
		_, err := c.services.Update(toUpdate)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) updatePods(key string, obj *corev1.Service, workloadIDs []string) error {
	// filter out project namespaces
	namespaces, err := dnsrecord.GetProjectNamespaces(c.namespaceLister, obj)
	if err != nil {
		return err
	}
	var podsToUpdate []*corev1.Pod
	set := labels.Set{}
	for key, val := range obj.Spec.Selector {
		set[key] = val
	}
	// reset the map
	targetWorkloadUUIDs := make(map[string]bool)
	for _, workloadID := range workloadIDs {
		groomed := strings.TrimSpace(workloadID)
		namespaceService := strings.Split(groomed, ":")
		if len(namespaceService) < 2 {
			return fmt.Errorf("Wrong format for workloadID [%s]", groomed)
		}
		namespace := namespaceService[0]
		if _, ok := namespaces[namespace]; !ok {
			logrus.Warnf("Failed to find namespace [%s] for workloadID [%s]", namespace, groomed)
			continue
		}
		workloadName := namespaceService[1]
		targetWorkload, err := c.deploymentLister.Get(namespace, workloadName)
		if err != nil {
			logrus.Warnf("Failed to fetch workload [%s]: [%v]", groomed, err)
			continue
		}
		if targetWorkload.DeletionTimestamp != nil {
			logrus.Warnf("Failed to fetch workload [%s]: workload is being removed", groomed)
			continue
		}

		// Add workload/deployment to the system map
		targetWorkloadUUID := fmt.Sprintf("%s/%s", targetWorkload.Namespace, targetWorkload.Name)
		targetWorkloadUUIDs[targetWorkloadUUID] = true

		// Find all the pods satisfying deployments' selectors
		set := labels.Set{}
		for key, val := range targetWorkload.Spec.Selector.MatchLabels {
			set[key] = val
		}
		workloadSelector := labels.SelectorFromSet(set)
		pods, err := c.podLister.List(targetWorkload.Namespace, workloadSelector)
		if err != nil {
			return errors.Wrapf(err, "Failed to list pods for target workload [%s]", groomed)
		}
		for _, pod := range pods {
			if pod.DeletionTimestamp != nil {
				continue
			}
			for svsSelectorKey, svcSelectorValue := range obj.Spec.Selector {
				if value, ok := pod.Labels[svsSelectorKey]; ok && value == svcSelectorValue {
					continue
				}
				podsToUpdate = append(podsToUpdate, pod)
			}
		}

		// Update the pods with the label
		for _, pod := range podsToUpdate {
			toUpdate := pod.DeepCopy()
			for svcSelectorKey, svcSelectorValue := range obj.Spec.Selector {
				toUpdate.Labels[svcSelectorKey] = svcSelectorValue
			}
			if _, err := c.pods.Update(toUpdate); err != nil {
				return errors.Wrapf(err, "Failed to update pod [%s] for target workload [%s]", pod.Name, groomed)
			}
		}
	}
	WorkloadServiceUUIDToDeploymentUUIDs.Store(key, targetWorkloadUUIDs)
	return nil
}

func getServiceSelector(obj *corev1.Service) string {
	return fmt.Sprintf("%s_%s", WorkloadIDLabelPrefix, obj.Name)
}
