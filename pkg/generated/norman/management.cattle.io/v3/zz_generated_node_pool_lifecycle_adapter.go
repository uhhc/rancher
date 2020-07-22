package v3

import (
	"github.com/rancher/norman/lifecycle"
	"github.com/rancher/norman/resource"
	"github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"k8s.io/apimachinery/pkg/runtime"
)

type NodePoolLifecycle interface {
	Create(obj *v3.NodePool) (runtime.Object, error)
	Remove(obj *v3.NodePool) (runtime.Object, error)
	Updated(obj *v3.NodePool) (runtime.Object, error)
}

type nodePoolLifecycleAdapter struct {
	lifecycle NodePoolLifecycle
}

func (w *nodePoolLifecycleAdapter) HasCreate() bool {
	o, ok := w.lifecycle.(lifecycle.ObjectLifecycleCondition)
	return !ok || o.HasCreate()
}

func (w *nodePoolLifecycleAdapter) HasFinalize() bool {
	o, ok := w.lifecycle.(lifecycle.ObjectLifecycleCondition)
	return !ok || o.HasFinalize()
}

func (w *nodePoolLifecycleAdapter) Create(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Create(obj.(*v3.NodePool))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *nodePoolLifecycleAdapter) Finalize(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Remove(obj.(*v3.NodePool))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *nodePoolLifecycleAdapter) Updated(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Updated(obj.(*v3.NodePool))
	if o == nil {
		return nil, err
	}
	return o, err
}

func NewNodePoolLifecycleAdapter(name string, clusterScoped bool, client NodePoolInterface, l NodePoolLifecycle) NodePoolHandlerFunc {
	if clusterScoped {
		resource.PutClusterScoped(NodePoolGroupVersionResource)
	}
	adapter := &nodePoolLifecycleAdapter{lifecycle: l}
	syncFn := lifecycle.NewObjectLifecycleAdapter(name, clusterScoped, adapter, client.ObjectClient())
	return func(key string, obj *v3.NodePool) (runtime.Object, error) {
		newObj, err := syncFn(key, obj)
		if o, ok := newObj.(runtime.Object); ok {
			return o, err
		}
		return nil, err
	}
}
