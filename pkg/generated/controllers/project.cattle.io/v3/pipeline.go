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

type PipelineHandler func(string, *v3.Pipeline) (*v3.Pipeline, error)

type PipelineController interface {
	generic.ControllerMeta
	PipelineClient

	OnChange(ctx context.Context, name string, sync PipelineHandler)
	OnRemove(ctx context.Context, name string, sync PipelineHandler)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, duration time.Duration)

	Cache() PipelineCache
}

type PipelineClient interface {
	Create(*v3.Pipeline) (*v3.Pipeline, error)
	Update(*v3.Pipeline) (*v3.Pipeline, error)
	UpdateStatus(*v3.Pipeline) (*v3.Pipeline, error)
	Delete(namespace, name string, options *metav1.DeleteOptions) error
	Get(namespace, name string, options metav1.GetOptions) (*v3.Pipeline, error)
	List(namespace string, opts metav1.ListOptions) (*v3.PipelineList, error)
	Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error)
	Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v3.Pipeline, err error)
}

type PipelineCache interface {
	Get(namespace, name string) (*v3.Pipeline, error)
	List(namespace string, selector labels.Selector) ([]*v3.Pipeline, error)

	AddIndexer(indexName string, indexer PipelineIndexer)
	GetByIndex(indexName, key string) ([]*v3.Pipeline, error)
}

type PipelineIndexer func(obj *v3.Pipeline) ([]string, error)

type pipelineController struct {
	controller    controller.SharedController
	client        *client.Client
	gvk           schema.GroupVersionKind
	groupResource schema.GroupResource
}

func NewPipelineController(gvk schema.GroupVersionKind, resource string, namespaced bool, controller controller.SharedControllerFactory) PipelineController {
	c := controller.ForResourceKind(gvk.GroupVersion().WithResource(resource), gvk.Kind, namespaced)
	return &pipelineController{
		controller: c,
		client:     c.Client(),
		gvk:        gvk,
		groupResource: schema.GroupResource{
			Group:    gvk.Group,
			Resource: resource,
		},
	}
}

func FromPipelineHandlerToHandler(sync PipelineHandler) generic.Handler {
	return func(key string, obj runtime.Object) (ret runtime.Object, err error) {
		var v *v3.Pipeline
		if obj == nil {
			v, err = sync(key, nil)
		} else {
			v, err = sync(key, obj.(*v3.Pipeline))
		}
		if v == nil {
			return nil, err
		}
		return v, err
	}
}

func (c *pipelineController) Updater() generic.Updater {
	return func(obj runtime.Object) (runtime.Object, error) {
		newObj, err := c.Update(obj.(*v3.Pipeline))
		if newObj == nil {
			return nil, err
		}
		return newObj, err
	}
}

func UpdatePipelineDeepCopyOnChange(client PipelineClient, obj *v3.Pipeline, handler func(obj *v3.Pipeline) (*v3.Pipeline, error)) (*v3.Pipeline, error) {
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

func (c *pipelineController) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	c.controller.RegisterHandler(ctx, name, controller.SharedControllerHandlerFunc(handler))
}

func (c *pipelineController) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), handler))
}

func (c *pipelineController) OnChange(ctx context.Context, name string, sync PipelineHandler) {
	c.AddGenericHandler(ctx, name, FromPipelineHandlerToHandler(sync))
}

func (c *pipelineController) OnRemove(ctx context.Context, name string, sync PipelineHandler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), FromPipelineHandlerToHandler(sync)))
}

func (c *pipelineController) Enqueue(namespace, name string) {
	c.controller.Enqueue(namespace, name)
}

func (c *pipelineController) EnqueueAfter(namespace, name string, duration time.Duration) {
	c.controller.EnqueueAfter(namespace, name, duration)
}

func (c *pipelineController) Informer() cache.SharedIndexInformer {
	return c.controller.Informer()
}

func (c *pipelineController) GroupVersionKind() schema.GroupVersionKind {
	return c.gvk
}

func (c *pipelineController) Cache() PipelineCache {
	return &pipelineCache{
		indexer:  c.Informer().GetIndexer(),
		resource: c.groupResource,
	}
}

func (c *pipelineController) Create(obj *v3.Pipeline) (*v3.Pipeline, error) {
	result := &v3.Pipeline{}
	return result, c.client.Create(context.TODO(), obj.Namespace, obj, result, metav1.CreateOptions{})
}

func (c *pipelineController) Update(obj *v3.Pipeline) (*v3.Pipeline, error) {
	result := &v3.Pipeline{}
	return result, c.client.Update(context.TODO(), obj.Namespace, obj, result, metav1.UpdateOptions{})
}

func (c *pipelineController) UpdateStatus(obj *v3.Pipeline) (*v3.Pipeline, error) {
	result := &v3.Pipeline{}
	return result, c.client.UpdateStatus(context.TODO(), obj.Namespace, obj, result, metav1.UpdateOptions{})
}

func (c *pipelineController) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	if options == nil {
		options = &metav1.DeleteOptions{}
	}
	return c.client.Delete(context.TODO(), namespace, name, *options)
}

func (c *pipelineController) Get(namespace, name string, options metav1.GetOptions) (*v3.Pipeline, error) {
	result := &v3.Pipeline{}
	return result, c.client.Get(context.TODO(), namespace, name, result, options)
}

func (c *pipelineController) List(namespace string, opts metav1.ListOptions) (*v3.PipelineList, error) {
	result := &v3.PipelineList{}
	return result, c.client.List(context.TODO(), namespace, result, opts)
}

func (c *pipelineController) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c.client.Watch(context.TODO(), namespace, opts)
}

func (c *pipelineController) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (*v3.Pipeline, error) {
	result := &v3.Pipeline{}
	return result, c.client.Patch(context.TODO(), namespace, name, pt, data, result, metav1.PatchOptions{}, subresources...)
}

type pipelineCache struct {
	indexer  cache.Indexer
	resource schema.GroupResource
}

func (c *pipelineCache) Get(namespace, name string) (*v3.Pipeline, error) {
	obj, exists, err := c.indexer.GetByKey(namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(c.resource, name)
	}
	return obj.(*v3.Pipeline), nil
}

func (c *pipelineCache) List(namespace string, selector labels.Selector) (ret []*v3.Pipeline, err error) {

	err = cache.ListAllByNamespace(c.indexer, namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v3.Pipeline))
	})

	return ret, err
}

func (c *pipelineCache) AddIndexer(indexName string, indexer PipelineIndexer) {
	utilruntime.Must(c.indexer.AddIndexers(map[string]cache.IndexFunc{
		indexName: func(obj interface{}) (strings []string, e error) {
			return indexer(obj.(*v3.Pipeline))
		},
	}))
}

func (c *pipelineCache) GetByIndex(indexName, key string) (result []*v3.Pipeline, err error) {
	objs, err := c.indexer.ByIndex(indexName, key)
	if err != nil {
		return nil, err
	}
	result = make([]*v3.Pipeline, 0, len(objs))
	for _, obj := range objs {
		result = append(result, obj.(*v3.Pipeline))
	}
	return result, nil
}

type PipelineStatusHandler func(obj *v3.Pipeline, status v3.PipelineStatus) (v3.PipelineStatus, error)

type PipelineGeneratingHandler func(obj *v3.Pipeline, status v3.PipelineStatus) ([]runtime.Object, v3.PipelineStatus, error)

func RegisterPipelineStatusHandler(ctx context.Context, controller PipelineController, condition condition.Cond, name string, handler PipelineStatusHandler) {
	statusHandler := &pipelineStatusHandler{
		client:    controller,
		condition: condition,
		handler:   handler,
	}
	controller.AddGenericHandler(ctx, name, FromPipelineHandlerToHandler(statusHandler.sync))
}

func RegisterPipelineGeneratingHandler(ctx context.Context, controller PipelineController, apply apply.Apply,
	condition condition.Cond, name string, handler PipelineGeneratingHandler, opts *generic.GeneratingHandlerOptions) {
	statusHandler := &pipelineGeneratingHandler{
		PipelineGeneratingHandler: handler,
		apply:                     apply,
		name:                      name,
		gvk:                       controller.GroupVersionKind(),
	}
	if opts != nil {
		statusHandler.opts = *opts
	}
	controller.OnChange(ctx, name, statusHandler.Remove)
	RegisterPipelineStatusHandler(ctx, controller, condition, name, statusHandler.Handle)
}

type pipelineStatusHandler struct {
	client    PipelineClient
	condition condition.Cond
	handler   PipelineStatusHandler
}

func (a *pipelineStatusHandler) sync(key string, obj *v3.Pipeline) (*v3.Pipeline, error) {
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
		if a.condition != "" {
			// Since status has changed, update the lastUpdatedTime
			a.condition.LastUpdated(&newStatus, time.Now().UTC().Format(time.RFC3339))
		}

		var newErr error
		obj.Status = newStatus
		obj, newErr = a.client.UpdateStatus(obj)
		if err == nil {
			err = newErr
		}
	}
	return obj, err
}

type pipelineGeneratingHandler struct {
	PipelineGeneratingHandler
	apply apply.Apply
	opts  generic.GeneratingHandlerOptions
	gvk   schema.GroupVersionKind
	name  string
}

func (a *pipelineGeneratingHandler) Remove(key string, obj *v3.Pipeline) (*v3.Pipeline, error) {
	if obj != nil {
		return obj, nil
	}

	obj = &v3.Pipeline{}
	obj.Namespace, obj.Name = kv.RSplit(key, "/")
	obj.SetGroupVersionKind(a.gvk)

	return nil, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects()
}

func (a *pipelineGeneratingHandler) Handle(obj *v3.Pipeline, status v3.PipelineStatus) (v3.PipelineStatus, error) {
	objs, newStatus, err := a.PipelineGeneratingHandler(obj, status)
	if err != nil {
		return newStatus, err
	}

	return newStatus, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects(objs...)
}
