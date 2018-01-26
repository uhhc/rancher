package controller

import (
	"context"

	"strings"

	"github.com/rancher/norman/clientbase"
	"github.com/rancher/norman/lifecycle"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

const (
	prtbByClusterIndex = "managment.cattle.io/prtb-by-cluster"
)

func registerClusterScopedGC(ctx context.Context, management *config.ManagementContext) {

	informer := management.Management.ProjectRoleTemplateBindings("").Controller().Informer()
	indexers := map[string]cache.IndexFunc{
		prtbByClusterIndex: prtbByCluster,
	}
	informer.AddIndexers(indexers)

	gc := &gcLifecycle{
		rtLister:      management.Management.RoleTemplates("").Controller().Lister(),
		projectLister: management.Management.Projects("").Controller().Lister(),
		crtbLister:    management.Management.ClusterRoleTemplateBindings("").Controller().Lister(),
		prtbIndexer:   informer.GetIndexer(),
		mgmt:          management,
	}

	management.Management.Clusters("").AddLifecycle("cluster-scoped-gc", gc)
}

type gcLifecycle struct {
	projectLister v3.ProjectLister
	crtbLister    v3.ClusterRoleTemplateBindingLister
	prtbIndexer   cache.Indexer
	rtLister      v3.RoleTemplateLister
	mgmt          *config.ManagementContext
}

func (c *gcLifecycle) Create(obj *v3.Cluster) (*v3.Cluster, error) {
	return obj, nil
}

func (c *gcLifecycle) Updated(obj *v3.Cluster) (*v3.Cluster, error) {
	return nil, nil
}

func cleanFinalizers(clusterName string, object runtime.Object, client *clientbase.ObjectClient) error {
	object = object.DeepCopyObject()
	modified := false
	md, err := meta.Accessor(object)
	if err != nil {
		return err
	}
	finalizers := md.GetFinalizers()
	for i := len(finalizers) - 1; i >= 0; i-- {
		f := finalizers[i]
		if strings.HasPrefix(f, lifecycle.ScopedFinalizerKey) && strings.HasSuffix(f, "_"+clusterName) {
			finalizers = append(finalizers[:i], finalizers[i+1:]...)
			modified = true
		}
	}

	if modified {
		md.SetFinalizers(finalizers)
		_, err := client.Update(md.GetName(), object)
		return err
	}
	return nil
}

func (c *gcLifecycle) Remove(cluster *v3.Cluster) (*v3.Cluster, error) {
	rts, err := c.rtLister.List("", labels.Everything())
	if err != nil {
		return cluster, err
	}
	oClient := c.mgmt.Management.RoleTemplates("").ObjectClient()
	for _, rt := range rts {
		if err := cleanFinalizers(cluster.Name, rt, oClient); err != nil {
			return cluster, err
		}
	}

	projects, err := c.projectLister.List(cluster.Name, labels.Everything())
	if err != nil {
		return cluster, err
	}
	oClient = c.mgmt.Management.Projects(cluster.Name).ObjectClient()
	for _, p := range projects {
		if err := cleanFinalizers(cluster.Name, p, oClient); err != nil {
			return cluster, err
		}
	}

	crtbs, err := c.crtbLister.List(cluster.Name, labels.Everything())
	if err != nil {
		return cluster, err
	}
	oClient = c.mgmt.Management.ClusterRoleTemplateBindings("").ObjectClient()
	for _, p := range crtbs {
		if err := cleanFinalizers(cluster.Name, p, oClient); err != nil {
			return cluster, err
		}
	}

	prtbs, err := c.prtbIndexer.ByIndex(prtbByClusterIndex, cluster.Name)
	if err != nil {
		return cluster, err
	}
	for _, pr := range prtbs {
		prtb, _ := pr.(*v3.ProjectRoleTemplateBinding)
		oClient = c.mgmt.Management.ProjectRoleTemplateBindings(prtb.Namespace).ObjectClient()
		if err := cleanFinalizers(cluster.Name, prtb, oClient); err != nil {
			return cluster, err
		}
	}

	return nil, nil
}

func prtbByCluster(obj interface{}) ([]string, error) {
	prtb, ok := obj.(*v3.ProjectRoleTemplateBinding)
	if !ok {
		return []string{}, nil
	}

	if parts := strings.SplitN(prtb.ProjectName, ":", 2); len(parts) == 2 && len(parts[1]) > 0 {
		return []string{parts[0]}, nil
	}
	return []string{}, nil
}
