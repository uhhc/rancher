package v3

import (
	"context"
	"time"

	"github.com/rancher/norman/controller"
	"github.com/rancher/norman/objectclient"
	"github.com/rancher/norman/resource"
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
	RkeAddonGroupVersionKind = schema.GroupVersionKind{
		Version: Version,
		Group:   GroupName,
		Kind:    "RkeAddon",
	}
	RkeAddonResource = metav1.APIResource{
		Name:         "rkeaddons",
		SingularName: "rkeaddon",
		Namespaced:   true,

		Kind: RkeAddonGroupVersionKind.Kind,
	}

	RkeAddonGroupVersionResource = schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "rkeaddons",
	}
)

func init() {
	resource.Put(RkeAddonGroupVersionResource)
}

func NewRkeAddon(namespace, name string, obj RkeAddon) *RkeAddon {
	obj.APIVersion, obj.Kind = RkeAddonGroupVersionKind.ToAPIVersionAndKind()
	obj.Name = name
	obj.Namespace = namespace
	return &obj
}

type RkeAddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RkeAddon `json:"items"`
}

type RkeAddonHandlerFunc func(key string, obj *RkeAddon) (runtime.Object, error)

type RkeAddonChangeHandlerFunc func(obj *RkeAddon) (runtime.Object, error)

type RkeAddonLister interface {
	List(namespace string, selector labels.Selector) (ret []*RkeAddon, err error)
	Get(namespace, name string) (*RkeAddon, error)
}

type RkeAddonController interface {
	Generic() controller.GenericController
	Informer() cache.SharedIndexInformer
	Lister() RkeAddonLister
	AddHandler(ctx context.Context, name string, handler RkeAddonHandlerFunc)
	AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync RkeAddonHandlerFunc)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, handler RkeAddonHandlerFunc)
	AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, handler RkeAddonHandlerFunc)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, after time.Duration)
}

type RkeAddonInterface interface {
	ObjectClient() *objectclient.ObjectClient
	Create(*RkeAddon) (*RkeAddon, error)
	GetNamespaced(namespace, name string, opts metav1.GetOptions) (*RkeAddon, error)
	Get(name string, opts metav1.GetOptions) (*RkeAddon, error)
	Update(*RkeAddon) (*RkeAddon, error)
	Delete(name string, options *metav1.DeleteOptions) error
	DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error
	List(opts metav1.ListOptions) (*RkeAddonList, error)
	ListNamespaced(namespace string, opts metav1.ListOptions) (*RkeAddonList, error)
	Watch(opts metav1.ListOptions) (watch.Interface, error)
	DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Controller() RkeAddonController
	AddHandler(ctx context.Context, name string, sync RkeAddonHandlerFunc)
	AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync RkeAddonHandlerFunc)
	AddLifecycle(ctx context.Context, name string, lifecycle RkeAddonLifecycle)
	AddFeatureLifecycle(ctx context.Context, enabled func() bool, name string, lifecycle RkeAddonLifecycle)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync RkeAddonHandlerFunc)
	AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, sync RkeAddonHandlerFunc)
	AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle RkeAddonLifecycle)
	AddClusterScopedFeatureLifecycle(ctx context.Context, enabled func() bool, name, clusterName string, lifecycle RkeAddonLifecycle)
}

type rkeAddonLister struct {
	controller *rkeAddonController
}

func (l *rkeAddonLister) List(namespace string, selector labels.Selector) (ret []*RkeAddon, err error) {
	err = cache.ListAllByNamespace(l.controller.Informer().GetIndexer(), namespace, selector, func(obj interface{}) {
		ret = append(ret, obj.(*RkeAddon))
	})
	return
}

func (l *rkeAddonLister) Get(namespace, name string) (*RkeAddon, error) {
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
			Group:    RkeAddonGroupVersionKind.Group,
			Resource: RkeAddonGroupVersionResource.Resource,
		}, key)
	}
	return obj.(*RkeAddon), nil
}

type rkeAddonController struct {
	controller.GenericController
}

func (c *rkeAddonController) Generic() controller.GenericController {
	return c.GenericController
}

func (c *rkeAddonController) Lister() RkeAddonLister {
	return &rkeAddonLister{
		controller: c,
	}
}

func (c *rkeAddonController) AddHandler(ctx context.Context, name string, handler RkeAddonHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*RkeAddon); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *rkeAddonController) AddFeatureHandler(ctx context.Context, enabled func() bool, name string, handler RkeAddonHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if !enabled() {
			return nil, nil
		} else if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*RkeAddon); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *rkeAddonController) AddClusterScopedHandler(ctx context.Context, name, cluster string, handler RkeAddonHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*RkeAddon); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *rkeAddonController) AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, cluster string, handler RkeAddonHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if !enabled() {
			return nil, nil
		} else if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*RkeAddon); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

type rkeAddonFactory struct {
}

func (c rkeAddonFactory) Object() runtime.Object {
	return &RkeAddon{}
}

func (c rkeAddonFactory) List() runtime.Object {
	return &RkeAddonList{}
}

func (s *rkeAddonClient) Controller() RkeAddonController {
	genericController := controller.NewGenericController(RkeAddonGroupVersionKind.Kind+"Controller",
		s.client.controllerFactory.ForResourceKind(RkeAddonGroupVersionResource, RkeAddonGroupVersionKind.Kind, true))

	return &rkeAddonController{
		GenericController: genericController,
	}
}

type rkeAddonClient struct {
	client       *Client
	ns           string
	objectClient *objectclient.ObjectClient
	controller   RkeAddonController
}

func (s *rkeAddonClient) ObjectClient() *objectclient.ObjectClient {
	return s.objectClient
}

func (s *rkeAddonClient) Create(o *RkeAddon) (*RkeAddon, error) {
	obj, err := s.objectClient.Create(o)
	return obj.(*RkeAddon), err
}

func (s *rkeAddonClient) Get(name string, opts metav1.GetOptions) (*RkeAddon, error) {
	obj, err := s.objectClient.Get(name, opts)
	return obj.(*RkeAddon), err
}

func (s *rkeAddonClient) GetNamespaced(namespace, name string, opts metav1.GetOptions) (*RkeAddon, error) {
	obj, err := s.objectClient.GetNamespaced(namespace, name, opts)
	return obj.(*RkeAddon), err
}

func (s *rkeAddonClient) Update(o *RkeAddon) (*RkeAddon, error) {
	obj, err := s.objectClient.Update(o.Name, o)
	return obj.(*RkeAddon), err
}

func (s *rkeAddonClient) UpdateStatus(o *RkeAddon) (*RkeAddon, error) {
	obj, err := s.objectClient.UpdateStatus(o.Name, o)
	return obj.(*RkeAddon), err
}

func (s *rkeAddonClient) Delete(name string, options *metav1.DeleteOptions) error {
	return s.objectClient.Delete(name, options)
}

func (s *rkeAddonClient) DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error {
	return s.objectClient.DeleteNamespaced(namespace, name, options)
}

func (s *rkeAddonClient) List(opts metav1.ListOptions) (*RkeAddonList, error) {
	obj, err := s.objectClient.List(opts)
	return obj.(*RkeAddonList), err
}

func (s *rkeAddonClient) ListNamespaced(namespace string, opts metav1.ListOptions) (*RkeAddonList, error) {
	obj, err := s.objectClient.ListNamespaced(namespace, opts)
	return obj.(*RkeAddonList), err
}

func (s *rkeAddonClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return s.objectClient.Watch(opts)
}

// Patch applies the patch and returns the patched deployment.
func (s *rkeAddonClient) Patch(o *RkeAddon, patchType types.PatchType, data []byte, subresources ...string) (*RkeAddon, error) {
	obj, err := s.objectClient.Patch(o.Name, o, patchType, data, subresources...)
	return obj.(*RkeAddon), err
}

func (s *rkeAddonClient) DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return s.objectClient.DeleteCollection(deleteOpts, listOpts)
}

func (s *rkeAddonClient) AddHandler(ctx context.Context, name string, sync RkeAddonHandlerFunc) {
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *rkeAddonClient) AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync RkeAddonHandlerFunc) {
	s.Controller().AddFeatureHandler(ctx, enabled, name, sync)
}

func (s *rkeAddonClient) AddLifecycle(ctx context.Context, name string, lifecycle RkeAddonLifecycle) {
	sync := NewRkeAddonLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *rkeAddonClient) AddFeatureLifecycle(ctx context.Context, enabled func() bool, name string, lifecycle RkeAddonLifecycle) {
	sync := NewRkeAddonLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddFeatureHandler(ctx, enabled, name, sync)
}

func (s *rkeAddonClient) AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync RkeAddonHandlerFunc) {
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *rkeAddonClient) AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, sync RkeAddonHandlerFunc) {
	s.Controller().AddClusterScopedFeatureHandler(ctx, enabled, name, clusterName, sync)
}

func (s *rkeAddonClient) AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle RkeAddonLifecycle) {
	sync := NewRkeAddonLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *rkeAddonClient) AddClusterScopedFeatureLifecycle(ctx context.Context, enabled func() bool, name, clusterName string, lifecycle RkeAddonLifecycle) {
	sync := NewRkeAddonLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedFeatureHandler(ctx, enabled, name, clusterName, sync)
}
