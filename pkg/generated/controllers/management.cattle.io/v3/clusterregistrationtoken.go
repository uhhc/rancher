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
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
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

type ClusterRegistrationTokenHandler func(string, *v3.ClusterRegistrationToken) (*v3.ClusterRegistrationToken, error)

type ClusterRegistrationTokenController interface {
	generic.ControllerMeta
	ClusterRegistrationTokenClient

	OnChange(ctx context.Context, name string, sync ClusterRegistrationTokenHandler)
	OnRemove(ctx context.Context, name string, sync ClusterRegistrationTokenHandler)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, duration time.Duration)

	Cache() ClusterRegistrationTokenCache
}

type ClusterRegistrationTokenClient interface {
	Create(*v3.ClusterRegistrationToken) (*v3.ClusterRegistrationToken, error)
	Update(*v3.ClusterRegistrationToken) (*v3.ClusterRegistrationToken, error)
	UpdateStatus(*v3.ClusterRegistrationToken) (*v3.ClusterRegistrationToken, error)
	Delete(namespace, name string, options *metav1.DeleteOptions) error
	Get(namespace, name string, options metav1.GetOptions) (*v3.ClusterRegistrationToken, error)
	List(namespace string, opts metav1.ListOptions) (*v3.ClusterRegistrationTokenList, error)
	Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error)
	Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v3.ClusterRegistrationToken, err error)
}

type ClusterRegistrationTokenCache interface {
	Get(namespace, name string) (*v3.ClusterRegistrationToken, error)
	List(namespace string, selector labels.Selector) ([]*v3.ClusterRegistrationToken, error)

	AddIndexer(indexName string, indexer ClusterRegistrationTokenIndexer)
	GetByIndex(indexName, key string) ([]*v3.ClusterRegistrationToken, error)
}

type ClusterRegistrationTokenIndexer func(obj *v3.ClusterRegistrationToken) ([]string, error)

type clusterRegistrationTokenController struct {
	controller    controller.SharedController
	client        *client.Client
	gvk           schema.GroupVersionKind
	groupResource schema.GroupResource
}

func NewClusterRegistrationTokenController(gvk schema.GroupVersionKind, resource string, namespaced bool, controller controller.SharedControllerFactory) ClusterRegistrationTokenController {
	c := controller.ForResourceKind(gvk.GroupVersion().WithResource(resource), gvk.Kind, namespaced)
	return &clusterRegistrationTokenController{
		controller: c,
		client:     c.Client(),
		gvk:        gvk,
		groupResource: schema.GroupResource{
			Group:    gvk.Group,
			Resource: resource,
		},
	}
}

func FromClusterRegistrationTokenHandlerToHandler(sync ClusterRegistrationTokenHandler) generic.Handler {
	return func(key string, obj runtime.Object) (ret runtime.Object, err error) {
		var v *v3.ClusterRegistrationToken
		if obj == nil {
			v, err = sync(key, nil)
		} else {
			v, err = sync(key, obj.(*v3.ClusterRegistrationToken))
		}
		if v == nil {
			return nil, err
		}
		return v, err
	}
}

func (c *clusterRegistrationTokenController) Updater() generic.Updater {
	return func(obj runtime.Object) (runtime.Object, error) {
		newObj, err := c.Update(obj.(*v3.ClusterRegistrationToken))
		if newObj == nil {
			return nil, err
		}
		return newObj, err
	}
}

func UpdateClusterRegistrationTokenDeepCopyOnChange(client ClusterRegistrationTokenClient, obj *v3.ClusterRegistrationToken, handler func(obj *v3.ClusterRegistrationToken) (*v3.ClusterRegistrationToken, error)) (*v3.ClusterRegistrationToken, error) {
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

func (c *clusterRegistrationTokenController) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	c.controller.RegisterHandler(ctx, name, controller.SharedControllerHandlerFunc(handler))
}

func (c *clusterRegistrationTokenController) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), handler))
}

func (c *clusterRegistrationTokenController) OnChange(ctx context.Context, name string, sync ClusterRegistrationTokenHandler) {
	c.AddGenericHandler(ctx, name, FromClusterRegistrationTokenHandlerToHandler(sync))
}

func (c *clusterRegistrationTokenController) OnRemove(ctx context.Context, name string, sync ClusterRegistrationTokenHandler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), FromClusterRegistrationTokenHandlerToHandler(sync)))
}

func (c *clusterRegistrationTokenController) Enqueue(namespace, name string) {
	c.controller.Enqueue(namespace, name)
}

func (c *clusterRegistrationTokenController) EnqueueAfter(namespace, name string, duration time.Duration) {
	c.controller.EnqueueAfter(namespace, name, duration)
}

func (c *clusterRegistrationTokenController) Informer() cache.SharedIndexInformer {
	return c.controller.Informer()
}

func (c *clusterRegistrationTokenController) GroupVersionKind() schema.GroupVersionKind {
	return c.gvk
}

func (c *clusterRegistrationTokenController) Cache() ClusterRegistrationTokenCache {
	return &clusterRegistrationTokenCache{
		indexer:  c.Informer().GetIndexer(),
		resource: c.groupResource,
	}
}

func (c *clusterRegistrationTokenController) Create(obj *v3.ClusterRegistrationToken) (*v3.ClusterRegistrationToken, error) {
	result := &v3.ClusterRegistrationToken{}
	return result, c.client.Create(context.TODO(), obj.Namespace, obj, result, metav1.CreateOptions{})
}

func (c *clusterRegistrationTokenController) Update(obj *v3.ClusterRegistrationToken) (*v3.ClusterRegistrationToken, error) {
	result := &v3.ClusterRegistrationToken{}
	return result, c.client.Update(context.TODO(), obj.Namespace, obj, result, metav1.UpdateOptions{})
}

func (c *clusterRegistrationTokenController) UpdateStatus(obj *v3.ClusterRegistrationToken) (*v3.ClusterRegistrationToken, error) {
	result := &v3.ClusterRegistrationToken{}
	return result, c.client.UpdateStatus(context.TODO(), obj.Namespace, obj, result, metav1.UpdateOptions{})
}

func (c *clusterRegistrationTokenController) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	if options == nil {
		options = &metav1.DeleteOptions{}
	}
	return c.client.Delete(context.TODO(), namespace, name, *options)
}

func (c *clusterRegistrationTokenController) Get(namespace, name string, options metav1.GetOptions) (*v3.ClusterRegistrationToken, error) {
	result := &v3.ClusterRegistrationToken{}
	return result, c.client.Get(context.TODO(), namespace, name, result, options)
}

func (c *clusterRegistrationTokenController) List(namespace string, opts metav1.ListOptions) (*v3.ClusterRegistrationTokenList, error) {
	result := &v3.ClusterRegistrationTokenList{}
	return result, c.client.List(context.TODO(), namespace, result, opts)
}

func (c *clusterRegistrationTokenController) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c.client.Watch(context.TODO(), namespace, opts)
}

func (c *clusterRegistrationTokenController) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (*v3.ClusterRegistrationToken, error) {
	result := &v3.ClusterRegistrationToken{}
	return result, c.client.Patch(context.TODO(), namespace, name, pt, data, result, metav1.PatchOptions{}, subresources...)
}

type clusterRegistrationTokenCache struct {
	indexer  cache.Indexer
	resource schema.GroupResource
}

func (c *clusterRegistrationTokenCache) Get(namespace, name string) (*v3.ClusterRegistrationToken, error) {
	obj, exists, err := c.indexer.GetByKey(namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(c.resource, name)
	}
	return obj.(*v3.ClusterRegistrationToken), nil
}

func (c *clusterRegistrationTokenCache) List(namespace string, selector labels.Selector) (ret []*v3.ClusterRegistrationToken, err error) {

	err = cache.ListAllByNamespace(c.indexer, namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v3.ClusterRegistrationToken))
	})

	return ret, err
}

func (c *clusterRegistrationTokenCache) AddIndexer(indexName string, indexer ClusterRegistrationTokenIndexer) {
	utilruntime.Must(c.indexer.AddIndexers(map[string]cache.IndexFunc{
		indexName: func(obj interface{}) (strings []string, e error) {
			return indexer(obj.(*v3.ClusterRegistrationToken))
		},
	}))
}

func (c *clusterRegistrationTokenCache) GetByIndex(indexName, key string) (result []*v3.ClusterRegistrationToken, err error) {
	objs, err := c.indexer.ByIndex(indexName, key)
	if err != nil {
		return nil, err
	}
	result = make([]*v3.ClusterRegistrationToken, 0, len(objs))
	for _, obj := range objs {
		result = append(result, obj.(*v3.ClusterRegistrationToken))
	}
	return result, nil
}

type ClusterRegistrationTokenStatusHandler func(obj *v3.ClusterRegistrationToken, status v3.ClusterRegistrationTokenStatus) (v3.ClusterRegistrationTokenStatus, error)

type ClusterRegistrationTokenGeneratingHandler func(obj *v3.ClusterRegistrationToken, status v3.ClusterRegistrationTokenStatus) ([]runtime.Object, v3.ClusterRegistrationTokenStatus, error)

func RegisterClusterRegistrationTokenStatusHandler(ctx context.Context, controller ClusterRegistrationTokenController, condition condition.Cond, name string, handler ClusterRegistrationTokenStatusHandler) {
	statusHandler := &clusterRegistrationTokenStatusHandler{
		client:    controller,
		condition: condition,
		handler:   handler,
	}
	controller.AddGenericHandler(ctx, name, FromClusterRegistrationTokenHandlerToHandler(statusHandler.sync))
}

func RegisterClusterRegistrationTokenGeneratingHandler(ctx context.Context, controller ClusterRegistrationTokenController, apply apply.Apply,
	condition condition.Cond, name string, handler ClusterRegistrationTokenGeneratingHandler, opts *generic.GeneratingHandlerOptions) {
	statusHandler := &clusterRegistrationTokenGeneratingHandler{
		ClusterRegistrationTokenGeneratingHandler: handler,
		apply: apply,
		name:  name,
		gvk:   controller.GroupVersionKind(),
	}
	if opts != nil {
		statusHandler.opts = *opts
	}
	controller.OnChange(ctx, name, statusHandler.Remove)
	RegisterClusterRegistrationTokenStatusHandler(ctx, controller, condition, name, statusHandler.Handle)
}

type clusterRegistrationTokenStatusHandler struct {
	client    ClusterRegistrationTokenClient
	condition condition.Cond
	handler   ClusterRegistrationTokenStatusHandler
}

func (a *clusterRegistrationTokenStatusHandler) sync(key string, obj *v3.ClusterRegistrationToken) (*v3.ClusterRegistrationToken, error) {
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

type clusterRegistrationTokenGeneratingHandler struct {
	ClusterRegistrationTokenGeneratingHandler
	apply apply.Apply
	opts  generic.GeneratingHandlerOptions
	gvk   schema.GroupVersionKind
	name  string
}

func (a *clusterRegistrationTokenGeneratingHandler) Remove(key string, obj *v3.ClusterRegistrationToken) (*v3.ClusterRegistrationToken, error) {
	if obj != nil {
		return obj, nil
	}

	obj = &v3.ClusterRegistrationToken{}
	obj.Namespace, obj.Name = kv.RSplit(key, "/")
	obj.SetGroupVersionKind(a.gvk)

	return nil, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects()
}

func (a *clusterRegistrationTokenGeneratingHandler) Handle(obj *v3.ClusterRegistrationToken, status v3.ClusterRegistrationTokenStatus) (v3.ClusterRegistrationTokenStatus, error) {
	objs, newStatus, err := a.ClusterRegistrationTokenGeneratingHandler(obj, status)
	if err != nil {
		return newStatus, err
	}

	return newStatus, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects(objs...)
}
