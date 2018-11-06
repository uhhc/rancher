package v3

import (
	"github.com/rancher/norman/lifecycle"
	"k8s.io/apimachinery/pkg/runtime"
)

type NamespacedCertificateLifecycle interface {
	Create(obj *NamespacedCertificate) (runtime.Object, error)
	Remove(obj *NamespacedCertificate) (runtime.Object, error)
	Updated(obj *NamespacedCertificate) (runtime.Object, error)
}

type namespacedCertificateLifecycleAdapter struct {
	lifecycle NamespacedCertificateLifecycle
}

func (w *namespacedCertificateLifecycleAdapter) Create(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Create(obj.(*NamespacedCertificate))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *namespacedCertificateLifecycleAdapter) Finalize(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Remove(obj.(*NamespacedCertificate))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *namespacedCertificateLifecycleAdapter) Updated(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Updated(obj.(*NamespacedCertificate))
	if o == nil {
		return nil, err
	}
	return o, err
}

func NewNamespacedCertificateLifecycleAdapter(name string, clusterScoped bool, client NamespacedCertificateInterface, l NamespacedCertificateLifecycle) NamespacedCertificateHandlerFunc {
	adapter := &namespacedCertificateLifecycleAdapter{lifecycle: l}
	syncFn := lifecycle.NewObjectLifecycleAdapter(name, clusterScoped, adapter, client.ObjectClient())
	return func(key string, obj *NamespacedCertificate) (runtime.Object, error) {
		newObj, err := syncFn(key, obj)
		if o, ok := newObj.(runtime.Object); ok {
			return o, err
		}
		return nil, err
	}
}
