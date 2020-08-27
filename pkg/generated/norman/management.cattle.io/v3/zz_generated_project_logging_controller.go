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
	ProjectLoggingGroupVersionKind = schema.GroupVersionKind{
		Version: Version,
		Group:   GroupName,
		Kind:    "ProjectLogging",
	}
	ProjectLoggingResource = metav1.APIResource{
		Name:         "projectloggings",
		SingularName: "projectlogging",
		Namespaced:   true,

		Kind: ProjectLoggingGroupVersionKind.Kind,
	}

	ProjectLoggingGroupVersionResource = schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "projectloggings",
	}
)

func init() {
	resource.Put(ProjectLoggingGroupVersionResource)
}

// Deprecated use v3.ProjectLogging instead
type ProjectLogging = v3.ProjectLogging

func NewProjectLogging(namespace, name string, obj v3.ProjectLogging) *v3.ProjectLogging {
	obj.APIVersion, obj.Kind = ProjectLoggingGroupVersionKind.ToAPIVersionAndKind()
	obj.Name = name
	obj.Namespace = namespace
	return &obj
}

type ProjectLoggingHandlerFunc func(key string, obj *v3.ProjectLogging) (runtime.Object, error)

type ProjectLoggingChangeHandlerFunc func(obj *v3.ProjectLogging) (runtime.Object, error)

type ProjectLoggingLister interface {
	List(namespace string, selector labels.Selector) (ret []*v3.ProjectLogging, err error)
	Get(namespace, name string) (*v3.ProjectLogging, error)
}

type ProjectLoggingController interface {
	Generic() controller.GenericController
	Informer() cache.SharedIndexInformer
	Lister() ProjectLoggingLister
	AddHandler(ctx context.Context, name string, handler ProjectLoggingHandlerFunc)
	AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync ProjectLoggingHandlerFunc)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, handler ProjectLoggingHandlerFunc)
	AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, handler ProjectLoggingHandlerFunc)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, after time.Duration)
}

type ProjectLoggingInterface interface {
	ObjectClient() *objectclient.ObjectClient
	Create(*v3.ProjectLogging) (*v3.ProjectLogging, error)
	GetNamespaced(namespace, name string, opts metav1.GetOptions) (*v3.ProjectLogging, error)
	Get(name string, opts metav1.GetOptions) (*v3.ProjectLogging, error)
	Update(*v3.ProjectLogging) (*v3.ProjectLogging, error)
	Delete(name string, options *metav1.DeleteOptions) error
	DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error
	List(opts metav1.ListOptions) (*v3.ProjectLoggingList, error)
	ListNamespaced(namespace string, opts metav1.ListOptions) (*v3.ProjectLoggingList, error)
	Watch(opts metav1.ListOptions) (watch.Interface, error)
	DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Controller() ProjectLoggingController
	AddHandler(ctx context.Context, name string, sync ProjectLoggingHandlerFunc)
	AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync ProjectLoggingHandlerFunc)
	AddLifecycle(ctx context.Context, name string, lifecycle ProjectLoggingLifecycle)
	AddFeatureLifecycle(ctx context.Context, enabled func() bool, name string, lifecycle ProjectLoggingLifecycle)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync ProjectLoggingHandlerFunc)
	AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, sync ProjectLoggingHandlerFunc)
	AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle ProjectLoggingLifecycle)
	AddClusterScopedFeatureLifecycle(ctx context.Context, enabled func() bool, name, clusterName string, lifecycle ProjectLoggingLifecycle)
}

type projectLoggingLister struct {
	ns         string
	controller *projectLoggingController
}

func (l *projectLoggingLister) List(namespace string, selector labels.Selector) (ret []*v3.ProjectLogging, err error) {
	if namespace == "" {
		namespace = l.ns
	}
	err = cache.ListAllByNamespace(l.controller.Informer().GetIndexer(), namespace, selector, func(obj interface{}) {
		ret = append(ret, obj.(*v3.ProjectLogging))
	})
	return
}

func (l *projectLoggingLister) Get(namespace, name string) (*v3.ProjectLogging, error) {
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
			Group:    ProjectLoggingGroupVersionKind.Group,
			Resource: ProjectLoggingGroupVersionResource.Resource,
		}, key)
	}
	return obj.(*v3.ProjectLogging), nil
}

type projectLoggingController struct {
	ns string
	controller.GenericController
}

func (c *projectLoggingController) Generic() controller.GenericController {
	return c.GenericController
}

func (c *projectLoggingController) Lister() ProjectLoggingLister {
	return &projectLoggingLister{
		ns:         c.ns,
		controller: c,
	}
}

func (c *projectLoggingController) AddHandler(ctx context.Context, name string, handler ProjectLoggingHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.ProjectLogging); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *projectLoggingController) AddFeatureHandler(ctx context.Context, enabled func() bool, name string, handler ProjectLoggingHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if !enabled() {
			return nil, nil
		} else if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.ProjectLogging); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *projectLoggingController) AddClusterScopedHandler(ctx context.Context, name, cluster string, handler ProjectLoggingHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.ProjectLogging); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *projectLoggingController) AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, cluster string, handler ProjectLoggingHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if !enabled() {
			return nil, nil
		} else if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*v3.ProjectLogging); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

type projectLoggingFactory struct {
}

func (c projectLoggingFactory) Object() runtime.Object {
	return &v3.ProjectLogging{}
}

func (c projectLoggingFactory) List() runtime.Object {
	return &v3.ProjectLoggingList{}
}

func (s *projectLoggingClient) Controller() ProjectLoggingController {
	genericController := controller.NewGenericController(s.ns, ProjectLoggingGroupVersionKind.Kind+"Controller",
		s.client.controllerFactory.ForResourceKind(ProjectLoggingGroupVersionResource, ProjectLoggingGroupVersionKind.Kind, true))

	return &projectLoggingController{
		ns:                s.ns,
		GenericController: genericController,
	}
}

type projectLoggingClient struct {
	client       *Client
	ns           string
	objectClient *objectclient.ObjectClient
	controller   ProjectLoggingController
}

func (s *projectLoggingClient) ObjectClient() *objectclient.ObjectClient {
	return s.objectClient
}

func (s *projectLoggingClient) Create(o *v3.ProjectLogging) (*v3.ProjectLogging, error) {
	obj, err := s.objectClient.Create(o)
	return obj.(*v3.ProjectLogging), err
}

func (s *projectLoggingClient) Get(name string, opts metav1.GetOptions) (*v3.ProjectLogging, error) {
	obj, err := s.objectClient.Get(name, opts)
	return obj.(*v3.ProjectLogging), err
}

func (s *projectLoggingClient) GetNamespaced(namespace, name string, opts metav1.GetOptions) (*v3.ProjectLogging, error) {
	obj, err := s.objectClient.GetNamespaced(namespace, name, opts)
	return obj.(*v3.ProjectLogging), err
}

func (s *projectLoggingClient) Update(o *v3.ProjectLogging) (*v3.ProjectLogging, error) {
	obj, err := s.objectClient.Update(o.Name, o)
	return obj.(*v3.ProjectLogging), err
}

func (s *projectLoggingClient) UpdateStatus(o *v3.ProjectLogging) (*v3.ProjectLogging, error) {
	obj, err := s.objectClient.UpdateStatus(o.Name, o)
	return obj.(*v3.ProjectLogging), err
}

func (s *projectLoggingClient) Delete(name string, options *metav1.DeleteOptions) error {
	return s.objectClient.Delete(name, options)
}

func (s *projectLoggingClient) DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error {
	return s.objectClient.DeleteNamespaced(namespace, name, options)
}

func (s *projectLoggingClient) List(opts metav1.ListOptions) (*v3.ProjectLoggingList, error) {
	obj, err := s.objectClient.List(opts)
	return obj.(*v3.ProjectLoggingList), err
}

func (s *projectLoggingClient) ListNamespaced(namespace string, opts metav1.ListOptions) (*v3.ProjectLoggingList, error) {
	obj, err := s.objectClient.ListNamespaced(namespace, opts)
	return obj.(*v3.ProjectLoggingList), err
}

func (s *projectLoggingClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return s.objectClient.Watch(opts)
}

// Patch applies the patch and returns the patched deployment.
func (s *projectLoggingClient) Patch(o *v3.ProjectLogging, patchType types.PatchType, data []byte, subresources ...string) (*v3.ProjectLogging, error) {
	obj, err := s.objectClient.Patch(o.Name, o, patchType, data, subresources...)
	return obj.(*v3.ProjectLogging), err
}

func (s *projectLoggingClient) DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return s.objectClient.DeleteCollection(deleteOpts, listOpts)
}

func (s *projectLoggingClient) AddHandler(ctx context.Context, name string, sync ProjectLoggingHandlerFunc) {
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *projectLoggingClient) AddFeatureHandler(ctx context.Context, enabled func() bool, name string, sync ProjectLoggingHandlerFunc) {
	s.Controller().AddFeatureHandler(ctx, enabled, name, sync)
}

func (s *projectLoggingClient) AddLifecycle(ctx context.Context, name string, lifecycle ProjectLoggingLifecycle) {
	sync := NewProjectLoggingLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *projectLoggingClient) AddFeatureLifecycle(ctx context.Context, enabled func() bool, name string, lifecycle ProjectLoggingLifecycle) {
	sync := NewProjectLoggingLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddFeatureHandler(ctx, enabled, name, sync)
}

func (s *projectLoggingClient) AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync ProjectLoggingHandlerFunc) {
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *projectLoggingClient) AddClusterScopedFeatureHandler(ctx context.Context, enabled func() bool, name, clusterName string, sync ProjectLoggingHandlerFunc) {
	s.Controller().AddClusterScopedFeatureHandler(ctx, enabled, name, clusterName, sync)
}

func (s *projectLoggingClient) AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle ProjectLoggingLifecycle) {
	sync := NewProjectLoggingLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *projectLoggingClient) AddClusterScopedFeatureLifecycle(ctx context.Context, enabled func() bool, name, clusterName string, lifecycle ProjectLoggingLifecycle) {
	sync := NewProjectLoggingLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedFeatureHandler(ctx, enabled, name, clusterName, sync)
}
