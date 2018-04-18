package rbac

import (
	"github.com/rancher/types/apis/management.cattle.io/v3"
	rbacv1 "github.com/rancher/types/apis/rbac.authorization.k8s.io/v1"
	"github.com/rancher/types/config"
	v12 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	grbByUserAndRoleIndex = "authz.cluster.cattle.io/grb-by-user-and-role"
)

func newGlobalRoleBindingHandler(workload *config.UserContext) *grbHandler {
	informer := workload.Management.Management.GlobalRoleBindings("").Controller().Informer()
	indexers := map[string]cache.IndexFunc{
		grbByUserAndRoleIndex: grbByUserAndRole,
	}
	informer.AddIndexers(indexers)

	return &grbHandler{
		grbIndexer:          informer.GetIndexer(),
		clusterRoleBindings: workload.RBAC.ClusterRoleBindings(""),
		crbLister:           workload.RBAC.ClusterRoleBindings("").Controller().Lister(),
	}
}

// grbHandler ensures the global admins have full access to every cluster. If a globalRoleBinding is created that uses
// the admin role, then the user in that binding gets a clusterRoleBinding in every user cluster to the cluster-admin role
type grbHandler struct {
	clusterRoleBindings rbacv1.ClusterRoleBindingInterface
	crbLister           rbacv1.ClusterRoleBindingLister
	grbIndexer          cache.Indexer
}

func (c *grbHandler) Create(obj *v3.GlobalRoleBinding) (*v3.GlobalRoleBinding, error) {
	if obj.GlobalRoleName != "admin" {
		return obj, nil
	}

	bindingName := grbCRBName(obj)
	b, err := c.crbLister.Get("", bindingName)
	if err != nil && !apierrors.IsNotFound(err) {
		return obj, err
	}

	if b != nil {
		// binding exists, nothing to do
		return obj, nil
	}

	_, err = c.clusterRoleBindings.Create(&v12.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: bindingName,
		},
		Subjects: []v12.Subject{
			{
				Kind: "User",
				Name: obj.UserName,
			},
		},
		RoleRef: v12.RoleRef{
			Name: "cluster-admin",
			Kind: "ClusterRole",
		},
	})
	return obj, err
}

func (c *grbHandler) Updated(obj *v3.GlobalRoleBinding) (*v3.GlobalRoleBinding, error) {
	return nil, nil
}

func (c *grbHandler) Remove(obj *v3.GlobalRoleBinding) (*v3.GlobalRoleBinding, error) {
	if obj.GlobalRoleName != "admin" {
		return obj, nil
	}

	grbs, err := c.grbIndexer.ByIndex(grbByUserAndRoleIndex, obj.UserName+"-"+obj.GlobalRoleName)
	if err != nil {
		return obj, err
	}

	if len(grbs) > 1 {
		return obj, nil
	}

	if err := c.clusterRoleBindings.Delete(grbCRBName(obj), nil); err != nil && !apierrors.IsNotFound(err) {
		return obj, err
	}

	return obj, nil
}

func grbCRBName(grb *v3.GlobalRoleBinding) string {
	return "globaladmin-" + grb.UserName
}

func grbByUserAndRole(obj interface{}) ([]string, error) {
	grb, ok := obj.(*v3.GlobalRoleBinding)
	if !ok {
		return []string{}, nil
	}

	return []string{grb.UserName + "-" + grb.GlobalRoleName}, nil
}
