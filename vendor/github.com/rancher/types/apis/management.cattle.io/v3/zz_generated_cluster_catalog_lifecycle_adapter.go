package v3

import (
	"github.com/rancher/norman/lifecycle"
	"k8s.io/apimachinery/pkg/runtime"
)

type ClusterCatalogLifecycle interface {
	Create(obj *ClusterCatalog) (*ClusterCatalog, error)
	Remove(obj *ClusterCatalog) (*ClusterCatalog, error)
	Updated(obj *ClusterCatalog) (*ClusterCatalog, error)
}

type clusterCatalogLifecycleAdapter struct {
	lifecycle ClusterCatalogLifecycle
}

func (w *clusterCatalogLifecycleAdapter) Create(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Create(obj.(*ClusterCatalog))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *clusterCatalogLifecycleAdapter) Finalize(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Remove(obj.(*ClusterCatalog))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *clusterCatalogLifecycleAdapter) Updated(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Updated(obj.(*ClusterCatalog))
	if o == nil {
		return nil, err
	}
	return o, err
}

func NewClusterCatalogLifecycleAdapter(name string, clusterScoped bool, client ClusterCatalogInterface, l ClusterCatalogLifecycle) ClusterCatalogHandlerFunc {
	adapter := &clusterCatalogLifecycleAdapter{lifecycle: l}
	syncFn := lifecycle.NewObjectLifecycleAdapter(name, clusterScoped, adapter, client.ObjectClient())
	return func(key string, obj *ClusterCatalog) (*ClusterCatalog, error) {
		newObj, err := syncFn(key, obj)
		if o, ok := newObj.(*ClusterCatalog); ok {
			return o, err
		}
		return nil, err
	}
}
