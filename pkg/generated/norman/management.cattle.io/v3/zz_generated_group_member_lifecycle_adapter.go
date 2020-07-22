package v3

import (
	"github.com/rancher/norman/lifecycle"
	"github.com/rancher/norman/resource"
	"github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"k8s.io/apimachinery/pkg/runtime"
)

type GroupMemberLifecycle interface {
	Create(obj *v3.GroupMember) (runtime.Object, error)
	Remove(obj *v3.GroupMember) (runtime.Object, error)
	Updated(obj *v3.GroupMember) (runtime.Object, error)
}

type groupMemberLifecycleAdapter struct {
	lifecycle GroupMemberLifecycle
}

func (w *groupMemberLifecycleAdapter) HasCreate() bool {
	o, ok := w.lifecycle.(lifecycle.ObjectLifecycleCondition)
	return !ok || o.HasCreate()
}

func (w *groupMemberLifecycleAdapter) HasFinalize() bool {
	o, ok := w.lifecycle.(lifecycle.ObjectLifecycleCondition)
	return !ok || o.HasFinalize()
}

func (w *groupMemberLifecycleAdapter) Create(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Create(obj.(*v3.GroupMember))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *groupMemberLifecycleAdapter) Finalize(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Remove(obj.(*v3.GroupMember))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *groupMemberLifecycleAdapter) Updated(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Updated(obj.(*v3.GroupMember))
	if o == nil {
		return nil, err
	}
	return o, err
}

func NewGroupMemberLifecycleAdapter(name string, clusterScoped bool, client GroupMemberInterface, l GroupMemberLifecycle) GroupMemberHandlerFunc {
	if clusterScoped {
		resource.PutClusterScoped(GroupMemberGroupVersionResource)
	}
	adapter := &groupMemberLifecycleAdapter{lifecycle: l}
	syncFn := lifecycle.NewObjectLifecycleAdapter(name, clusterScoped, adapter, client.ObjectClient())
	return func(key string, obj *v3.GroupMember) (runtime.Object, error) {
		newObj, err := syncFn(key, obj)
		if o, ok := newObj.(runtime.Object); ok {
			return o, err
		}
		return nil, err
	}
}
