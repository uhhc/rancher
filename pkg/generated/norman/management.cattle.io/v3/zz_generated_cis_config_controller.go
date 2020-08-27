package v3

import (
	"context"
	"time"

	"github.com/rancher/norman/controller"
	"github.com/rancher/norman/objectclient"
	"github.com/rancher/norman/resource"
	"github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

var (
	CisConfigGroupVersionKind = schema.GroupVersionKind{
		Version: Version,
		Group:   GroupName,
		Kind:    "CisConfig",
	}
	CisConfigResource = metav1.APIResource{
		Name:         "cisconfigs",
		SingularName: "cisconfig",
		Namespaced:   true,

		Kind: CisConfigGroupVersionKind.Kind,
	}

	CisConfigGroupVersionResource = schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "cisconfigs",
	}
)

func init() {
	resource.Put(CisConfigGroupVersionResource)
}

// Deprecated use v3.CisConfig instead
type CisConfig = v3.CisConfig

func NewCisConfig(namespace, name string, obj v3.CisConfig) *v3.CisConfig {
	obj.APIVersion, obj.Kind = CisConfigGroupVersionKind.ToAPIVersionAndKind()
	obj.Name = name
	obj.Namespace = namespace
	return &obj
}

type CisConfigHandlerFunc func(key string, obj *v3.CisConfig) (runtime.Object, error)

type CisConfigChangeHandlerFunc func(obj *v3.CisConfig) (runtime.Object, error)

type CisConfigLister interface {
	List(namespace string, selector labels.Selector) (ret []*v3.CisConfig, err error)
	Get(namespace, name string) (*v3.CisConfig, error)
}

type CisConfigController interface {
	Generic() controller.GenericController
	Informer() cache.SharedIndexInformer
	Lister() CisConfigLister
	AddHandler(ctx context.Context, name string, handler CisConfigHandlerFunc)
	AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync CisConfigHandlerFunc)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, handler CisConfigHandlerFunc)
	AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, handler CisConfigHandlerFunc)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, after time.Duration)
}

type CisConfigInterface interface {
	ObjectClient() *objectclient.ObjectClient
	Create(*v3.CisConfig) (*v3.CisConfig, error)
	GetNamespaced(namespace, name string, opts metav1.GetOptions) (*v3.CisConfig, error)
	Get(name string, opts metav1.GetOptions) (*v3.CisConfig, error)
	Update(*v3.CisConfig) (*v3.CisConfig, error)
	Delete(name string, options *metav1.DeleteOptions) error
	DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error
	List(opts metav1.ListOptions) (*v3.CisConfigList, error)
	ListNamespaced(namespace string, opts metav1.ListOptions) (*v3.CisConfigList, error)
	Watch(opts metav1.ListOptions) (watch.Interface, error)
	DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Controller() CisConfigController
	AddHandler(ctx context.Context, name string, sync CisConfigHandlerFunc)
	AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync CisConfigHandlerFunc)
	AddLifecycle(ctx context.Context, name string, lifecycle CisConfigLifecycle)
	AddFeatureLifecycle(ctx context.Context, enabled func() bool, name string, lifecycle CisConfigLifecycle)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync CisConfigHandlerFunc)
	AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, sync CisConfigHandlerFunc)
	AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle CisConfigLifecycle)
	AddClusterScopedFeatureLifecycle(ctx context.Context, enabled func() bool, name, clusterName string, lifecycle CisConfigLifecycle)
}

type cisConfigLister struct {
	ns         string
	controller *cisConfigController
}

func (l *cisConfigLister) List(namespace string, selector labels.Selector) (ret []*v3.CisConfig, err error) {
	if namespace == "" {
		namespace = l.ns
	}
	err = cache.ListAllByNamespace(l.controller.Informer().GetIndexer(), namespace, selector, func(obj interface{}) {
		ret = append(ret, obj.(*v3.CisConfig))
	})
	return
}

func (l *cisConfigLister) Get(namespace, name string) (*v3.CisConfig, error) {
	var key string
	if namespace != "" {
		key = namespace + "/" + name
	} else {
		key = name
	}
	obj, exists, err := l.controller.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    CisConfigGroupVersionKind.Group,
			Resource: CisConfigGroupVersionResource.Resource,
		}, key)
	}
	return obj.(*v3.CisConfig), nil
}

type cisConfigController struct {
	ns string
	controller.GenericController
}

func (c *cisConfigController) Generic() controller.GenericController {
	return c.GenericController
}

func (c *cisConfigController) Lister() CisConfigLister {
	return &cisConfigLister{
		ns:         c.ns,
		controller: c,
	}
}

func (c *cisConfigController) AddHandler(ctx context.Context, name string, handler CisConfigHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.CisConfig); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *cisConfigController) AddFeatureHandler(ctx context.Context, enabled func() bool, name string, handler CisConfigHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if !enabled() {
			return nil, nil
		} else if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.CisConfig); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *cisConfigController) AddClusterScopedHandler(ctx context.Context, name, cluster string, handler CisConfigHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.CisConfig); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *cisConfigController) AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, cluster string, handler CisConfigHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if !enabled() {
			return nil, nil
		} else if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.CisConfig); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

type cisConfigFactory struct {
}

func (c cisConfigFactory) Object() runtime.Object {
	return &v3.CisConfig{}
}

func (c cisConfigFactory) List() runtime.Object {
	return &v3.CisConfigList{}
}

func (s *cisConfigClient) Controller() CisConfigController {
	genericController := controller.NewGenericController(s.ns, CisConfigGroupVersionKind.Kind+"Controller",
		s.client.controllerFactory.ForResourceKind(CisConfigGroupVersionResource, CisConfigGroupVersionKind.Kind, true))

	return &cisConfigController{
		ns:                s.ns,
		GenericController: genericController,
	}
}

type cisConfigClient struct {
	client       *Client
	ns           string
	objectClient *objectclient.ObjectClient
	controller   CisConfigController
}

func (s *cisConfigClient) ObjectClient() *objectclient.ObjectClient {
	return s.objectClient
}

func (s *cisConfigClient) Create(o *v3.CisConfig) (*v3.CisConfig, error) {
	obj, err := s.objectClient.Create(o)
	return obj.(*v3.CisConfig), err
}

func (s *cisConfigClient) Get(name string, opts metav1.GetOptions) (*v3.CisConfig, error) {
	obj, err := s.objectClient.Get(name, opts)
	return obj.(*v3.CisConfig), err
}

func (s *cisConfigClient) GetNamespaced(namespace, name string, opts metav1.GetOptions) (*v3.CisConfig, error) {
	obj, err := s.objectClient.GetNamespaced(namespace, name, opts)
	return obj.(*v3.CisConfig), err
}

func (s *cisConfigClient) Update(o *v3.CisConfig) (*v3.CisConfig, error) {
	obj, err := s.objectClient.Update(o.Name, o)
	return obj.(*v3.CisConfig), err
}

func (s *cisConfigClient) UpdateStatus(o *v3.CisConfig) (*v3.CisConfig, error) {
	obj, err := s.objectClient.UpdateStatus(o.Name, o)
	return obj.(*v3.CisConfig), err
}

func (s *cisConfigClient) Delete(name string, options *metav1.DeleteOptions) error {
	return s.objectClient.Delete(name, options)
}

func (s *cisConfigClient) DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error {
	return s.objectClient.DeleteNamespaced(namespace, name, options)
}

func (s *cisConfigClient) List(opts metav1.ListOptions) (*v3.CisConfigList, error) {
	obj, err := s.objectClient.List(opts)
	return obj.(*v3.CisConfigList), err
}

func (s *cisConfigClient) ListNamespaced(namespace string, opts metav1.ListOptions) (*v3.CisConfigList, error) {
	obj, err := s.objectClient.ListNamespaced(namespace, opts)
	return obj.(*v3.CisConfigList), err
}

func (s *cisConfigClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return s.objectClient.Watch(opts)
}

// Patch applies the patch and returns the patched deployment.
func (s *cisConfigClient) Patch(o *v3.CisConfig, patchType types.PatchType, data []byte, subresources ...string) (*v3.CisConfig, error) {
	obj, err := s.objectClient.Patch(o.Name, o, patchType, data, subresources...)
	return obj.(*v3.CisConfig), err
}

func (s *cisConfigClient) DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return s.objectClient.DeleteCollection(deleteOpts, listOpts)
}

func (s *cisConfigClient) AddHandler(ctx context.Context, name string, sync CisConfigHandlerFunc) {
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *cisConfigClient) AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync CisConfigHandlerFunc) {
	s.Controller().AddFeatureHandler(ctx, enabled, name, sync)
}

func (s *cisConfigClient) AddLifecycle(ctx context.Context, name string, lifecycle CisConfigLifecycle) {
	sync := NewCisConfigLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *cisConfigClient) AddFeatureLifecycle(ctx context.Context, enabled func() bool, name string, lifecycle CisConfigLifecycle) {
	sync := NewCisConfigLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddFeatureHandler(ctx, enabled, name, sync)
}

func (s *cisConfigClient) AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync CisConfigHandlerFunc) {
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *cisConfigClient) AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, sync CisConfigHandlerFunc) {
	s.Controller().AddClusterScopedFeatureHandler(ctx, enabled, name, clusterName, sync)
}

func (s *cisConfigClient) AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle CisConfigLifecycle) {
	sync := NewCisConfigLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *cisConfigClient) AddClusterScopedFeatureLifecycle(ctx context.Context, enabled func() bool, name, clusterName string, lifecycle CisConfigLifecycle) {
	sync := NewCisConfigLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedFeatureHandler(ctx, enabled, name, clusterName, sync)
}
