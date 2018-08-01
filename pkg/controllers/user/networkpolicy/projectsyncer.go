package networkpolicy

import (
	"fmt"
	"reflect"

	"github.com/rancher/norman/condition"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

type projectSyncer struct {
	pnpLister        v3.ProjectNetworkPolicyLister
	pnpClient        v3.ProjectNetworkPolicyInterface
	projClient       v3.ProjectInterface
	clusterLister    v3.ClusterLister
	clusterNamespace string
}

// Sync is responsible for creating a default ProjectNetworkPolicy for
// every project created. There is no need to worry about clean up, as
// this pnp object is tied to the namespace of the project, it's deleted
// automatically.
func (ps *projectSyncer) Sync(key string, p *v3.Project) error {
	if p == nil || p.DeletionTimestamp != nil {
		return nil
	}
	disabled, err := isNetworkPolicyDisabled(ps.clusterNamespace, ps.clusterLister)
	if err != nil {
		return err
	}
	if disabled {
		return nil
	}
	updated, err := ps.createDefaultNetworkPolicy(p)
	if err != nil {
		return err
	}

	// update if it has changed
	if updated != nil && !reflect.DeepEqual(p, updated) {
		_, err = ps.projClient.Update(updated)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ps *projectSyncer) createDefaultNetworkPolicy(p *v3.Project) (*v3.Project, error) {
	updated, err := v3.DefaultNetworkPolicyCreated.Do(p, func() (runtime.Object, error) {
		o, err := meta.Accessor(p)
		if err != nil {
			return p, condition.Error("MissingMetadata", err)
		}

		projectName := o.GetName()
		defaultPolicyName := "pnp-" + projectName
		existingPolicies, err := ps.pnpLister.List(defaultPolicyName, labels.Everything())
		if err != nil {
			logrus.Errorf("projectSyncer: createDefaultNetworkPolicy: error fetching existing project network policy: %v", err)
			return p, err
		}
		if len(existingPolicies) == 0 {
			pnpDesc := fmt.Sprintf("Default network policy for project %v", projectName)
			_, err = ps.pnpClient.Create(&v3.ProjectNetworkPolicy{
				ObjectMeta: v1.ObjectMeta{
					Name:      defaultPolicyName,
					Namespace: projectName,
				},
				Spec: v3.ProjectNetworkPolicySpec{
					Description: pnpDesc,
					ProjectName: o.GetNamespace() + ":" + projectName,
				},
			})
			if err == nil {
				logrus.Infof("projectSyncer: createDefaultNetworkPolicy: successfully created default network policy for project: %v", projectName)
			}
		}

		return p, nil
	})
	if err != nil {
		return p, err
	}
	return updated.(*v3.Project), nil
}
