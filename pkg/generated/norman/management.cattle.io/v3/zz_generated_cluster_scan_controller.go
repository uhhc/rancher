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
	ClusterScanGroupVersionKind = schema.GroupVersionKind{
		Version: Version,
		Group:   GroupName,
		Kind:    "ClusterScan",
	}
	ClusterScanResource = metav1.APIResource{
		Name:         "clusterscans",
		SingularName: "clusterscan",
		Namespaced:   true,

		Kind: ClusterScanGroupVersionKind.Kind,
	}

	ClusterScanGroupVersionResource = schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "clusterscans",
	}
)

func init() {
	resource.Put(ClusterScanGroupVersionResource)
}

// Deprecated use v3.ClusterScan instead
type ClusterScan = v3.ClusterScan

func NewClusterScan(namespace, name string, obj v3.ClusterScan) *v3.ClusterScan {
	obj.APIVersion, obj.Kind = ClusterScanGroupVersionKind.ToAPIVersionAndKind()
	obj.Name = name
	obj.Namespace = namespace
	return &obj
}

type ClusterScanHandlerFunc func(key string, obj *v3.ClusterScan) (runtime.Object, error)

type ClusterScanChangeHandlerFunc func(obj *v3.ClusterScan) (runtime.Object, error)

type ClusterScanLister interface {
	List(namespace string, selector labels.Selector) (ret []*v3.ClusterScan, err error)
	Get(namespace, name string) (*v3.ClusterScan, error)
}

type ClusterScanController interface {
	Generic() controller.GenericController
	Informer() cache.SharedIndexInformer
	Lister() ClusterScanLister
	AddHandler(ctx context.Context, name string, handler ClusterScanHandlerFunc)
	AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync ClusterScanHandlerFunc)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, handler ClusterScanHandlerFunc)
	AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, handler ClusterScanHandlerFunc)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, after time.Duration)
}

type ClusterScanInterface interface {
	ObjectClient() *objectclient.ObjectClient
	Create(*v3.ClusterScan) (*v3.ClusterScan, error)
	GetNamespaced(namespace, name string, opts metav1.GetOptions) (*v3.ClusterScan, error)
	Get(name string, opts metav1.GetOptions) (*v3.ClusterScan, error)
	Update(*v3.ClusterScan) (*v3.ClusterScan, error)
	Delete(name string, options *metav1.DeleteOptions) error
	DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error
	List(opts metav1.ListOptions) (*v3.ClusterScanList, error)
	ListNamespaced(namespace string, opts metav1.ListOptions) (*v3.ClusterScanList, error)
	Watch(opts metav1.ListOptions) (watch.Interface, error)
	DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Controller() ClusterScanController
	AddHandler(ctx context.Context, name string, sync ClusterScanHandlerFunc)
	AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync ClusterScanHandlerFunc)
	AddLifecycle(ctx context.Context, name string, lifecycle ClusterScanLifecycle)
	AddFeatureLifecycle(ctx context.Context, enabled func() bool, name string, lifecycle ClusterScanLifecycle)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync ClusterScanHandlerFunc)
	AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, sync ClusterScanHandlerFunc)
	AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle ClusterScanLifecycle)
	AddClusterScopedFeatureLifecycle(ctx context.Context, enabled func() bool, name, clusterName string, lifecycle ClusterScanLifecycle)
}

type clusterScanLister struct {
	ns         string
	controller *clusterScanController
}

func (l *clusterScanLister) List(namespace string, selector labels.Selector) (ret []*v3.ClusterScan, err error) {
	if namespace == "" {
		namespace = l.ns
	}
	err = cache.ListAllByNamespace(l.controller.Informer().GetIndexer(), namespace, selector, func(obj interface{}) {
		ret = append(ret, obj.(*v3.ClusterScan))
	})
	return
}

func (l *clusterScanLister) Get(namespace, name string) (*v3.ClusterScan, error) {
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
			Group:    ClusterScanGroupVersionKind.Group,
			Resource: ClusterScanGroupVersionResource.Resource,
		}, key)
	}
	return obj.(*v3.ClusterScan), nil
}

type clusterScanController struct {
	ns string
	controller.GenericController
}

func (c *clusterScanController) Generic() controller.GenericController {
	return c.GenericController
}

func (c *clusterScanController) Lister() ClusterScanLister {
	return &clusterScanLister{
		ns:         c.ns,
		controller: c,
	}
}

func (c *clusterScanController) AddHandler(ctx context.Context, name string, handler ClusterScanHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.ClusterScan); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *clusterScanController) AddFeatureHandler(ctx context.Context, enabled func() bool, name string, handler ClusterScanHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if !enabled() {
			return nil, nil
		} else if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.ClusterScan); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *clusterScanController) AddClusterScopedHandler(ctx context.Context, name, cluster string, handler ClusterScanHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.ClusterScan); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *clusterScanController) AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, cluster string, handler ClusterScanHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if !enabled() {
			return nil, nil
		} else if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.ClusterScan); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

type clusterScanFactory struct {
}

func (c clusterScanFactory) Object() runtime.Object {
	return &v3.ClusterScan{}
}

func (c clusterScanFactory) List() runtime.Object {
	return &v3.ClusterScanList{}
}

func (s *clusterScanClient) Controller() ClusterScanController {
	genericController := controller.NewGenericController(s.ns, ClusterScanGroupVersionKind.Kind+"Controller",
		s.client.controllerFactory.ForResourceKind(ClusterScanGroupVersionResource, ClusterScanGroupVersionKind.Kind, true))

	return &clusterScanController{
		ns:                s.ns,
		GenericController: genericController,
	}
}

type clusterScanClient struct {
	client       *Client
	ns           string
	objectClient *objectclient.ObjectClient
	controller   ClusterScanController
}

func (s *clusterScanClient) ObjectClient() *objectclient.ObjectClient {
	return s.objectClient
}

func (s *clusterScanClient) Create(o *v3.ClusterScan) (*v3.ClusterScan, error) {
	obj, err := s.objectClient.Create(o)
	return obj.(*v3.ClusterScan), err
}

func (s *clusterScanClient) Get(name string, opts metav1.GetOptions) (*v3.ClusterScan, error) {
	obj, err := s.objectClient.Get(name, opts)
	return obj.(*v3.ClusterScan), err
}

func (s *clusterScanClient) GetNamespaced(namespace, name string, opts metav1.GetOptions) (*v3.ClusterScan, error) {
	obj, err := s.objectClient.GetNamespaced(namespace, name, opts)
	return obj.(*v3.ClusterScan), err
}

func (s *clusterScanClient) Update(o *v3.ClusterScan) (*v3.ClusterScan, error) {
	obj, err := s.objectClient.Update(o.Name, o)
	return obj.(*v3.ClusterScan), err
}

func (s *clusterScanClient) UpdateStatus(o *v3.ClusterScan) (*v3.ClusterScan, error) {
	obj, err := s.objectClient.UpdateStatus(o.Name, o)
	return obj.(*v3.ClusterScan), err
}

func (s *clusterScanClient) Delete(name string, options *metav1.DeleteOptions) error {
	return s.objectClient.Delete(name, options)
}

func (s *clusterScanClient) DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error {
	return s.objectClient.DeleteNamespaced(namespace, name, options)
}

func (s *clusterScanClient) List(opts metav1.ListOptions) (*v3.ClusterScanList, error) {
	obj, err := s.objectClient.List(opts)
	return obj.(*v3.ClusterScanList), err
}

func (s *clusterScanClient) ListNamespaced(namespace string, opts metav1.ListOptions) (*v3.ClusterScanList, error) {
	obj, err := s.objectClient.ListNamespaced(namespace, opts)
	return obj.(*v3.ClusterScanList), err
}

func (s *clusterScanClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return s.objectClient.Watch(opts)
}

// Patch applies the patch and returns the patched deployment.
func (s *clusterScanClient) Patch(o *v3.ClusterScan, patchType types.PatchType, data []byte, subresources ...string) (*v3.ClusterScan, error) {
	obj, err := s.objectClient.Patch(o.Name, o, patchType, data, subresources...)
	return obj.(*v3.ClusterScan), err
}

func (s *clusterScanClient) DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return s.objectClient.DeleteCollection(deleteOpts, listOpts)
}

func (s *clusterScanClient) AddHandler(ctx context.Context, name string, sync ClusterScanHandlerFunc) {
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *clusterScanClient) AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync ClusterScanHandlerFunc) {
	s.Controller().AddFeatureHandler(ctx, enabled, name, sync)
}

func (s *clusterScanClient) AddLifecycle(ctx context.Context, name string, lifecycle ClusterScanLifecycle) {
	sync := NewClusterScanLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *clusterScanClient) AddFeatureLifecycle(ctx context.Context, enabled func() bool, name string, lifecycle ClusterScanLifecycle) {
	sync := NewClusterScanLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddFeatureHandler(ctx, enabled, name, sync)
}

func (s *clusterScanClient) AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync ClusterScanHandlerFunc) {
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *clusterScanClient) AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, sync ClusterScanHandlerFunc) {
	s.Controller().AddClusterScopedFeatureHandler(ctx, enabled, name, clusterName, sync)
}

func (s *clusterScanClient) AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle ClusterScanLifecycle) {
	sync := NewClusterScanLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *clusterScanClient) AddClusterScopedFeatureLifecycle(ctx context.Context, enabled func() bool, name, clusterName string, lifecycle ClusterScanLifecycle) {
	sync := NewClusterScanLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedFeatureHandler(ctx, enabled, name, clusterName, sync)
}
