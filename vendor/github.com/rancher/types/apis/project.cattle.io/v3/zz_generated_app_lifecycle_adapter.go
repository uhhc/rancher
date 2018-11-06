package v3

import (
	"github.com/rancher/norman/lifecycle"
	"k8s.io/apimachinery/pkg/runtime"
)

type AppLifecycle interface {
	Create(obj *App) (runtime.Object, error)
	Remove(obj *App) (runtime.Object, error)
	Updated(obj *App) (runtime.Object, error)
}

type appLifecycleAdapter struct {
	lifecycle AppLifecycle
}

func (w *appLifecycleAdapter) Create(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Create(obj.(*App))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *appLifecycleAdapter) Finalize(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Remove(obj.(*App))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *appLifecycleAdapter) Updated(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Updated(obj.(*App))
	if o == nil {
		return nil, err
	}
	return o, err
}

func NewAppLifecycleAdapter(name string, clusterScoped bool, client AppInterface, l AppLifecycle) AppHandlerFunc {
	adapter := &appLifecycleAdapter{lifecycle: l}
	syncFn := lifecycle.NewObjectLifecycleAdapter(name, clusterScoped, adapter, client.ObjectClient())
	return func(key string, obj *App) (runtime.Object, error) {
		newObj, err := syncFn(key, obj)
		if o, ok := newObj.(runtime.Object); ok {
			return o, err
		}
		return nil, err
	}
}
