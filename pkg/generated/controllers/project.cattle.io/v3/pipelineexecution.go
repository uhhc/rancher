/*
Copyright 2020 Rancher Labs, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by main. DO NOT EDIT.

package v3

import (
	"context"
	"time"

	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	v3 "github.com/rancher/rancher/pkg/apis/project.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/condition"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/kv"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

type PipelineExecutionHandler func(string, *v3.PipelineExecution) (*v3.PipelineExecution, error)

type PipelineExecutionController interface {
	generic.ControllerMeta
	PipelineExecutionClient

	OnChange(ctx context.Context, name string, sync PipelineExecutionHandler)
	OnRemove(ctx context.Context, name string, sync PipelineExecutionHandler)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, duration time.Duration)

	Cache() PipelineExecutionCache
}

type PipelineExecutionClient interface {
	Create(*v3.PipelineExecution) (*v3.PipelineExecution, error)
	Update(*v3.PipelineExecution) (*v3.PipelineExecution, error)
	UpdateStatus(*v3.PipelineExecution) (*v3.PipelineExecution, error)
	Delete(namespace, name string, options *metav1.DeleteOptions) error
	Get(namespace, name string, options metav1.GetOptions) (*v3.PipelineExecution, error)
	List(namespace string, opts metav1.ListOptions) (*v3.PipelineExecutionList, error)
	Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error)
	Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v3.PipelineExecution, err error)
}

type PipelineExecutionCache interface {
	Get(namespace, name string) (*v3.PipelineExecution, error)
	List(namespace string, selector labels.Selector) ([]*v3.PipelineExecution, error)

	AddIndexer(indexName string, indexer PipelineExecutionIndexer)
	GetByIndex(indexName, key string) ([]*v3.PipelineExecution, error)
}

type PipelineExecutionIndexer func(obj *v3.PipelineExecution) ([]string, error)

type pipelineExecutionController struct {
	controller    controller.SharedController
	client        *client.Client
	gvk           schema.GroupVersionKind
	groupResource schema.GroupResource
}

func NewPipelineExecutionController(gvk schema.GroupVersionKind, resource string, namespaced bool, controller controller.SharedControllerFactory) PipelineExecutionController {
	c := controller.ForResourceKind(gvk.GroupVersion().WithResource(resource), gvk.Kind, namespaced)
	return &pipelineExecutionController{
		controller: c,
		client:     c.Client(),
		gvk:        gvk,
		groupResource: schema.GroupResource{
			Group:    gvk.Group,
			Resource: resource,
		},
	}
}

func FromPipelineExecutionHandlerToHandler(sync PipelineExecutionHandler) generic.Handler {
	return func(key string, obj runtime.Object) (ret runtime.Object, err error) {
		var v *v3.PipelineExecution
		if obj == nil {
			v, err = sync(key, nil)
		} else {
			v, err = sync(key, obj.(*v3.PipelineExecution))
		}
		if v == nil {
			return nil, err
		}
		return v, err
	}
}

func (c *pipelineExecutionController) Updater() generic.Updater {
	return func(obj runtime.Object) (runtime.Object, error) {
		newObj, err := c.Update(obj.(*v3.PipelineExecution))
		if newObj == nil {
			return nil, err
		}
		return newObj, err
	}
}

func UpdatePipelineExecutionDeepCopyOnChange(client PipelineExecutionClient, obj *v3.PipelineExecution, handler func(obj *v3.PipelineExecution) (*v3.PipelineExecution, error)) (*v3.PipelineExecution, error) {
	if obj == nil {
		return obj, nil
	}

	copyObj := obj.DeepCopy()
	newObj, err := handler(copyObj)
	if newObj != nil {
		copyObj = newObj
	}
	if obj.ResourceVersion == copyObj.ResourceVersion && !equality.Semantic.DeepEqual(obj, copyObj) {
		return client.Update(copyObj)
	}

	return copyObj, err
}

func (c *pipelineExecutionController) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	c.controller.RegisterHandler(ctx, name, controller.SharedControllerHandlerFunc(handler))
}

func (c *pipelineExecutionController) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), handler))
}

func (c *pipelineExecutionController) OnChange(ctx context.Context, name string, sync PipelineExecutionHandler) {
	c.AddGenericHandler(ctx, name, FromPipelineExecutionHandlerToHandler(sync))
}

func (c *pipelineExecutionController) OnRemove(ctx context.Context, name string, sync PipelineExecutionHandler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), FromPipelineExecutionHandlerToHandler(sync)))
}

func (c *pipelineExecutionController) Enqueue(namespace, name string) {
	c.controller.Enqueue(namespace, name)
}

func (c *pipelineExecutionController) EnqueueAfter(namespace, name string, duration time.Duration) {
	c.controller.EnqueueAfter(namespace, name, duration)
}

func (c *pipelineExecutionController) Informer() cache.SharedIndexInformer {
	return c.controller.Informer()
}

func (c *pipelineExecutionController) GroupVersionKind() schema.GroupVersionKind {
	return c.gvk
}

func (c *pipelineExecutionController) Cache() PipelineExecutionCache {
	return &pipelineExecutionCache{
		indexer:  c.Informer().GetIndexer(),
		resource: c.groupResource,
	}
}

func (c *pipelineExecutionController) Create(obj *v3.PipelineExecution) (*v3.PipelineExecution, error) {
	result := &v3.PipelineExecution{}
	return result, c.client.Create(context.TODO(), obj.Namespace, obj, result, metav1.CreateOptions{})
}

func (c *pipelineExecutionController) Update(obj *v3.PipelineExecution) (*v3.PipelineExecution, error) {
	result := &v3.PipelineExecution{}
	return result, c.client.Update(context.TODO(), obj.Namespace, obj, result, metav1.UpdateOptions{})
}

func (c *pipelineExecutionController) UpdateStatus(obj *v3.PipelineExecution) (*v3.PipelineExecution, error) {
	result := &v3.PipelineExecution{}
	return result, c.client.UpdateStatus(context.TODO(), obj.Namespace, obj, result, metav1.UpdateOptions{})
}

func (c *pipelineExecutionController) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	if options == nil {
		options = &metav1.DeleteOptions{}
	}
	return c.client.Delete(context.TODO(), namespace, name, *options)
}

func (c *pipelineExecutionController) Get(namespace, name string, options metav1.GetOptions) (*v3.PipelineExecution, error) {
	result := &v3.PipelineExecution{}
	return result, c.client.Get(context.TODO(), namespace, name, result, options)
}

func (c *pipelineExecutionController) List(namespace string, opts metav1.ListOptions) (*v3.PipelineExecutionList, error) {
	result := &v3.PipelineExecutionList{}
	return result, c.client.List(context.TODO(), namespace, result, opts)
}

func (c *pipelineExecutionController) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c.client.Watch(context.TODO(), namespace, opts)
}

func (c *pipelineExecutionController) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (*v3.PipelineExecution, error) {
	result := &v3.PipelineExecution{}
	return result, c.client.Patch(context.TODO(), namespace, name, pt, data, result, metav1.PatchOptions{}, subresources...)
}

type pipelineExecutionCache struct {
	indexer  cache.Indexer
	resource schema.GroupResource
}

func (c *pipelineExecutionCache) Get(namespace, name string) (*v3.PipelineExecution, error) {
	obj, exists, err := c.indexer.GetByKey(namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(c.resource, name)
	}
	return obj.(*v3.PipelineExecution), nil
}

func (c *pipelineExecutionCache) List(namespace string, selector labels.Selector) (ret []*v3.PipelineExecution, err error) {

	err = cache.ListAllByNamespace(c.indexer, namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v3.PipelineExecution))
	})

	return ret, err
}

func (c *pipelineExecutionCache) AddIndexer(indexName string, indexer PipelineExecutionIndexer) {
	utilruntime.Must(c.indexer.AddIndexers(map[string]cache.IndexFunc{
		indexName: func(obj interface{}) (strings []string, e error) {
			return indexer(obj.(*v3.PipelineExecution))
		},
	}))
}

func (c *pipelineExecutionCache) GetByIndex(indexName, key string) (result []*v3.PipelineExecution, err error) {
	objs, err := c.indexer.ByIndex(indexName, key)
	if err != nil {
		return nil, err
	}
	result = make([]*v3.PipelineExecution, 0, len(objs))
	for _, obj := range objs {
		result = append(result, obj.(*v3.PipelineExecution))
	}
	return result, nil
}

type PipelineExecutionStatusHandler func(obj *v3.PipelineExecution, status v3.PipelineExecutionStatus) (v3.PipelineExecutionStatus, error)

type PipelineExecutionGeneratingHandler func(obj *v3.PipelineExecution, status v3.PipelineExecutionStatus) ([]runtime.Object, v3.PipelineExecutionStatus, error)

func RegisterPipelineExecutionStatusHandler(ctx context.Context, controller PipelineExecutionController, condition condition.Cond, name string, handler PipelineExecutionStatusHandler) {
	statusHandler := &pipelineExecutionStatusHandler{
		client:    controller,
		condition: condition,
		handler:   handler,
	}
	controller.AddGenericHandler(ctx, name, FromPipelineExecutionHandlerToHandler(statusHandler.sync))
}

func RegisterPipelineExecutionGeneratingHandler(ctx context.Context, controller PipelineExecutionController, apply apply.Apply,
	condition condition.Cond, name string, handler PipelineExecutionGeneratingHandler, opts *generic.GeneratingHandlerOptions) {
	statusHandler := &pipelineExecutionGeneratingHandler{
		PipelineExecutionGeneratingHandler: handler,
		apply:                              apply,
		name:                               name,
		gvk:                                controller.GroupVersionKind(),
	}
	if opts != nil {
		statusHandler.opts = *opts
	}
	controller.OnChange(ctx, name, statusHandler.Remove)
	RegisterPipelineExecutionStatusHandler(ctx, controller, condition, name, statusHandler.Handle)
}

type pipelineExecutionStatusHandler struct {
	client    PipelineExecutionClient
	condition condition.Cond
	handler   PipelineExecutionStatusHandler
}

func (a *pipelineExecutionStatusHandler) sync(key string, obj *v3.PipelineExecution) (*v3.PipelineExecution, error) {
	if obj == nil {
		return obj, nil
	}

	origStatus := obj.Status.DeepCopy()
	obj = obj.DeepCopy()
	newStatus, err := a.handler(obj, obj.Status)
	if err != nil {
		// Revert to old status on error
		newStatus = *origStatus.DeepCopy()
	}

	if a.condition != "" {
		if errors.IsConflict(err) {
			a.condition.SetError(&newStatus, "", nil)
		} else {
			a.condition.SetError(&newStatus, "", err)
		}
	}
	if !equality.Semantic.DeepEqual(origStatus, &newStatus) {
		var newErr error
		obj.Status = newStatus
		obj, newErr = a.client.UpdateStatus(obj)
		if err == nil {
			err = newErr
		}
	}
	return obj, err
}

type pipelineExecutionGeneratingHandler struct {
	PipelineExecutionGeneratingHandler
	apply apply.Apply
	opts  generic.GeneratingHandlerOptions
	gvk   schema.GroupVersionKind
	name  string
}

func (a *pipelineExecutionGeneratingHandler) Remove(key string, obj *v3.PipelineExecution) (*v3.PipelineExecution, error) {
	if obj != nil {
		return obj, nil
	}

	obj = &v3.PipelineExecution{}
	obj.Namespace, obj.Name = kv.RSplit(key, "/")
	obj.SetGroupVersionKind(a.gvk)

	return nil, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects()
}

func (a *pipelineExecutionGeneratingHandler) Handle(obj *v3.PipelineExecution, status v3.PipelineExecutionStatus) (v3.PipelineExecutionStatus, error) {
	objs, newStatus, err := a.PipelineExecutionGeneratingHandler(obj, status)
	if err != nil {
		return newStatus, err
	}

	return newStatus, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects(objs...)
}
