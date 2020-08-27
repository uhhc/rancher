package v3

import (
	"context"
	"time"

	"github.com/rancher/norman/controller"
	"github.com/rancher/norman/objectclient"
	"github.com/rancher/norman/resource"
	"github.com/rancher/rancher/pkg/apis/project.cattle.io/v3"
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
	SourceCodeRepositoryGroupVersionKind = schema.GroupVersionKind{
		Version: Version,
		Group:   GroupName,
		Kind:    "SourceCodeRepository",
	}
	SourceCodeRepositoryResource = metav1.APIResource{
		Name:         "sourcecoderepositories",
		SingularName: "sourcecoderepository",
		Namespaced:   true,

		Kind: SourceCodeRepositoryGroupVersionKind.Kind,
	}

	SourceCodeRepositoryGroupVersionResource = schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "sourcecoderepositories",
	}
)

func init() {
	resource.Put(SourceCodeRepositoryGroupVersionResource)
}

// Deprecated use v3.SourceCodeRepository instead
type SourceCodeRepository = v3.SourceCodeRepository

func NewSourceCodeRepository(namespace, name string, obj v3.SourceCodeRepository) *v3.SourceCodeRepository {
	obj.APIVersion, obj.Kind = SourceCodeRepositoryGroupVersionKind.ToAPIVersionAndKind()
	obj.Name = name
	obj.Namespace = namespace
	return &obj
}

type SourceCodeRepositoryHandlerFunc func(key string, obj *v3.SourceCodeRepository) (runtime.Object, error)

type SourceCodeRepositoryChangeHandlerFunc func(obj *v3.SourceCodeRepository) (runtime.Object, error)

type SourceCodeRepositoryLister interface {
	List(namespace string, selector labels.Selector) (ret []*v3.SourceCodeRepository, err error)
	Get(namespace, name string) (*v3.SourceCodeRepository, error)
}

type SourceCodeRepositoryController interface {
	Generic() controller.GenericController
	Informer() cache.SharedIndexInformer
	Lister() SourceCodeRepositoryLister
	AddHandler(ctx context.Context, name string, handler SourceCodeRepositoryHandlerFunc)
	AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync SourceCodeRepositoryHandlerFunc)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, handler SourceCodeRepositoryHandlerFunc)
	AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, handler SourceCodeRepositoryHandlerFunc)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, after time.Duration)
}

type SourceCodeRepositoryInterface interface {
	ObjectClient() *objectclient.ObjectClient
	Create(*v3.SourceCodeRepository) (*v3.SourceCodeRepository, error)
	GetNamespaced(namespace, name string, opts metav1.GetOptions) (*v3.SourceCodeRepository, error)
	Get(name string, opts metav1.GetOptions) (*v3.SourceCodeRepository, error)
	Update(*v3.SourceCodeRepository) (*v3.SourceCodeRepository, error)
	Delete(name string, options *metav1.DeleteOptions) error
	DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error
	List(opts metav1.ListOptions) (*v3.SourceCodeRepositoryList, error)
	ListNamespaced(namespace string, opts metav1.ListOptions) (*v3.SourceCodeRepositoryList, error)
	Watch(opts metav1.ListOptions) (watch.Interface, error)
	DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Controller() SourceCodeRepositoryController
	AddHandler(ctx context.Context, name string, sync SourceCodeRepositoryHandlerFunc)
	AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync SourceCodeRepositoryHandlerFunc)
	AddLifecycle(ctx context.Context, name string, lifecycle SourceCodeRepositoryLifecycle)
	AddFeatureLifecycle(ctx context.Context, enabled func() bool, name string, lifecycle SourceCodeRepositoryLifecycle)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync SourceCodeRepositoryHandlerFunc)
	AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, sync SourceCodeRepositoryHandlerFunc)
	AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle SourceCodeRepositoryLifecycle)
	AddClusterScopedFeatureLifecycle(ctx context.Context, enabled func() bool, name, clusterName string, lifecycle SourceCodeRepositoryLifecycle)
}

type sourceCodeRepositoryLister struct {
	ns         string
	controller *sourceCodeRepositoryController
}

func (l *sourceCodeRepositoryLister) List(namespace string, selector labels.Selector) (ret []*v3.SourceCodeRepository, err error) {
	if namespace == "" {
		namespace = l.ns
	}
	err = cache.ListAllByNamespace(l.controller.Informer().GetIndexer(), namespace, selector, func(obj interface{}) {
		ret = append(ret, obj.(*v3.SourceCodeRepository))
	})
	return
}

func (l *sourceCodeRepositoryLister) Get(namespace, name string) (*v3.SourceCodeRepository, error) {
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
			Group:    SourceCodeRepositoryGroupVersionKind.Group,
			Resource: SourceCodeRepositoryGroupVersionResource.Resource,
		}, key)
	}
	return obj.(*v3.SourceCodeRepository), nil
}

type sourceCodeRepositoryController struct {
	ns string
	controller.GenericController
}

func (c *sourceCodeRepositoryController) Generic() controller.GenericController {
	return c.GenericController
}

func (c *sourceCodeRepositoryController) Lister() SourceCodeRepositoryLister {
	return &sourceCodeRepositoryLister{
		ns:         c.ns,
		controller: c,
	}
}

func (c *sourceCodeRepositoryController) AddHandler(ctx context.Context, name string, handler SourceCodeRepositoryHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.SourceCodeRepository); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *sourceCodeRepositoryController) AddFeatureHandler(ctx context.Context, enabled func() bool, name string, handler SourceCodeRepositoryHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if !enabled() {
			return nil, nil
		} else if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.SourceCodeRepository); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *sourceCodeRepositoryController) AddClusterScopedHandler(ctx context.Context, name, cluster string, handler SourceCodeRepositoryHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.SourceCodeRepository); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *sourceCodeRepositoryController) AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, cluster string, handler SourceCodeRepositoryHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if !enabled() {
			return nil, nil
		} else if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.SourceCodeRepository); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

type sourceCodeRepositoryFactory struct {
}

func (c sourceCodeRepositoryFactory) Object() runtime.Object {
	return &v3.SourceCodeRepository{}
}

func (c sourceCodeRepositoryFactory) List() runtime.Object {
	return &v3.SourceCodeRepositoryList{}
}

func (s *sourceCodeRepositoryClient) Controller() SourceCodeRepositoryController {
	genericController := controller.NewGenericController(s.ns, SourceCodeRepositoryGroupVersionKind.Kind+"Controller",
		s.client.controllerFactory.ForResourceKind(SourceCodeRepositoryGroupVersionResource, SourceCodeRepositoryGroupVersionKind.Kind, true))

	return &sourceCodeRepositoryController{
		ns:                s.ns,
		GenericController: genericController,
	}
}

type sourceCodeRepositoryClient struct {
	client       *Client
	ns           string
	objectClient *objectclient.ObjectClient
	controller   SourceCodeRepositoryController
}

func (s *sourceCodeRepositoryClient) ObjectClient() *objectclient.ObjectClient {
	return s.objectClient
}

func (s *sourceCodeRepositoryClient) Create(o *v3.SourceCodeRepository) (*v3.SourceCodeRepository, error) {
	obj, err := s.objectClient.Create(o)
	return obj.(*v3.SourceCodeRepository), err
}

func (s *sourceCodeRepositoryClient) Get(name string, opts metav1.GetOptions) (*v3.SourceCodeRepository, error) {
	obj, err := s.objectClient.Get(name, opts)
	return obj.(*v3.SourceCodeRepository), err
}

func (s *sourceCodeRepositoryClient) GetNamespaced(namespace, name string, opts metav1.GetOptions) (*v3.SourceCodeRepository, error) {
	obj, err := s.objectClient.GetNamespaced(namespace, name, opts)
	return obj.(*v3.SourceCodeRepository), err
}

func (s *sourceCodeRepositoryClient) Update(o *v3.SourceCodeRepository) (*v3.SourceCodeRepository, error) {
	obj, err := s.objectClient.Update(o.Name, o)
	return obj.(*v3.SourceCodeRepository), err
}

func (s *sourceCodeRepositoryClient) UpdateStatus(o *v3.SourceCodeRepository) (*v3.SourceCodeRepository, error) {
	obj, err := s.objectClient.UpdateStatus(o.Name, o)
	return obj.(*v3.SourceCodeRepository), err
}

func (s *sourceCodeRepositoryClient) Delete(name string, options *metav1.DeleteOptions) error {
	return s.objectClient.Delete(name, options)
}

func (s *sourceCodeRepositoryClient) DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error {
	return s.objectClient.DeleteNamespaced(namespace, name, options)
}

func (s *sourceCodeRepositoryClient) List(opts metav1.ListOptions) (*v3.SourceCodeRepositoryList, error) {
	obj, err := s.objectClient.List(opts)
	return obj.(*v3.SourceCodeRepositoryList), err
}

func (s *sourceCodeRepositoryClient) ListNamespaced(namespace string, opts metav1.ListOptions) (*v3.SourceCodeRepositoryList, error) {
	obj, err := s.objectClient.ListNamespaced(namespace, opts)
	return obj.(*v3.SourceCodeRepositoryList), err
}

func (s *sourceCodeRepositoryClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return s.objectClient.Watch(opts)
}

// Patch applies the patch and returns the patched deployment.
func (s *sourceCodeRepositoryClient) Patch(o *v3.SourceCodeRepository, patchType types.PatchType, data []byte, subresources ...string) (*v3.SourceCodeRepository, error) {
	obj, err := s.objectClient.Patch(o.Name, o, patchType, data, subresources...)
	return obj.(*v3.SourceCodeRepository), err
}

func (s *sourceCodeRepositoryClient) DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return s.objectClient.DeleteCollection(deleteOpts, listOpts)
}

func (s *sourceCodeRepositoryClient) AddHandler(ctx context.Context, name string, sync SourceCodeRepositoryHandlerFunc) {
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *sourceCodeRepositoryClient) AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync SourceCodeRepositoryHandlerFunc) {
	s.Controller().AddFeatureHandler(ctx, enabled, name, sync)
}

func (s *sourceCodeRepositoryClient) AddLifecycle(ctx context.Context, name string, lifecycle SourceCodeRepositoryLifecycle) {
	sync := NewSourceCodeRepositoryLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *sourceCodeRepositoryClient) AddFeatureLifecycle(ctx context.Context, enabled func() bool, name string, lifecycle SourceCodeRepositoryLifecycle) {
	sync := NewSourceCodeRepositoryLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddFeatureHandler(ctx, enabled, name, sync)
}

func (s *sourceCodeRepositoryClient) AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync SourceCodeRepositoryHandlerFunc) {
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *sourceCodeRepositoryClient) AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, sync SourceCodeRepositoryHandlerFunc) {
	s.Controller().AddClusterScopedFeatureHandler(ctx, enabled, name, clusterName, sync)
}

func (s *sourceCodeRepositoryClient) AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle SourceCodeRepositoryLifecycle) {
	sync := NewSourceCodeRepositoryLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *sourceCodeRepositoryClient) AddClusterScopedFeatureLifecycle(ctx context.Context, enabled func() bool, name, clusterName string, lifecycle SourceCodeRepositoryLifecycle) {
	sync := NewSourceCodeRepositoryLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedFeatureHandler(ctx, enabled, name, clusterName, sync)
}
