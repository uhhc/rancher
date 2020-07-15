package v3

import (
	"github.com/rancher/norman/lifecycle"
	"github.com/rancher/norman/resource"
	"github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"k8s.io/apimachinery/pkg/runtime"
)

type ProjectLoggingLifecycle interface {
	Create(obj *v3.ProjectLogging) (runtime.Object, error)
	Remove(obj *v3.ProjectLogging) (runtime.Object, error)
	Updated(obj *v3.ProjectLogging) (runtime.Object, error)
}

type projectLoggingLifecycleAdapter struct {
	lifecycle ProjectLoggingLifecycle
}

func (w *projectLoggingLifecycleAdapter) HasCreate() bool {
	o, ok := w.lifecycle.(lifecycle.ObjectLifecycleCondition)
	return !ok || o.HasCreate()
}

func (w *projectLoggingLifecycleAdapter) HasFinalize() bool {
	o, ok := w.lifecycle.(lifecycle.ObjectLifecycleCondition)
	return !ok || o.HasFinalize()
}

func (w *projectLoggingLifecycleAdapter) Create(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Create(obj.(*v3.ProjectLogging))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *projectLoggingLifecycleAdapter) Finalize(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Remove(obj.(*v3.ProjectLogging))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *projectLoggingLifecycleAdapter) Updated(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Updated(obj.(*v3.ProjectLogging))
	if o == nil {
		return nil, err
	}
	return o, err
}

func NewProjectLoggingLifecycleAdapter(name string, clusterScoped bool, client ProjectLoggingInterface, l ProjectLoggingLifecycle) ProjectLoggingHandlerFunc {
	if clusterScoped {
		resource.PutClusterScoped(ProjectLoggingGroupVersionResource)
	}
	adapter := &projectLoggingLifecycleAdapter{lifecycle: l}
	syncFn := lifecycle.NewObjectLifecycleAdapter(name, clusterScoped, adapter, client.ObjectClient())
	return func(key string, obj *v3.ProjectLogging) (runtime.Object, error) {
		newObj, err := syncFn(key, obj)
		if o, ok := newObj.(runtime.Object); ok {
			return o, err
		}
		return nil, err
	}
}
