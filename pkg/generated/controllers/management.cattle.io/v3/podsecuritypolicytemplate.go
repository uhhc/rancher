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
	"github.com/rancher/wrangler/pkg/generic"
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

type PodSecurityPolicyTemplateHandler func(string, *v3.PodSecurityPolicyTemplate) (*v3.PodSecurityPolicyTemplate, error)

type PodSecurityPolicyTemplateController interface {
	generic.ControllerMeta
	PodSecurityPolicyTemplateClient

	OnChange(ctx context.Context, name string, sync PodSecurityPolicyTemplateHandler)
	OnRemove(ctx context.Context, name string, sync PodSecurityPolicyTemplateHandler)
	Enqueue(name string)
	EnqueueAfter(name string, duration time.Duration)

	Cache() PodSecurityPolicyTemplateCache
}

type PodSecurityPolicyTemplateClient interface {
	Create(*v3.PodSecurityPolicyTemplate) (*v3.PodSecurityPolicyTemplate, error)
	Update(*v3.PodSecurityPolicyTemplate) (*v3.PodSecurityPolicyTemplate, error)

	Delete(name string, options *metav1.DeleteOptions) error
	Get(name string, options metav1.GetOptions) (*v3.PodSecurityPolicyTemplate, error)
	List(opts metav1.ListOptions) (*v3.PodSecurityPolicyTemplateList, error)
	Watch(opts metav1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v3.PodSecurityPolicyTemplate, err error)
}

type PodSecurityPolicyTemplateCache interface {
	Get(name string) (*v3.PodSecurityPolicyTemplate, error)
	List(selector labels.Selector) ([]*v3.PodSecurityPolicyTemplate, error)

	AddIndexer(indexName string, indexer PodSecurityPolicyTemplateIndexer)
	GetByIndex(indexName, key string) ([]*v3.PodSecurityPolicyTemplate, error)
}

type PodSecurityPolicyTemplateIndexer func(obj *v3.PodSecurityPolicyTemplate) ([]string, error)

type podSecurityPolicyTemplateController struct {
	controller    controller.SharedController
	client        *client.Client
	gvk           schema.GroupVersionKind
	groupResource schema.GroupResource
}

func NewPodSecurityPolicyTemplateController(gvk schema.GroupVersionKind, resource string, namespaced bool, controller controller.SharedControllerFactory) PodSecurityPolicyTemplateController {
	c := controller.ForResourceKind(gvk.GroupVersion().WithResource(resource), gvk.Kind, namespaced)
	return &podSecurityPolicyTemplateController{
		controller: c,
		client:     c.Client(),
		gvk:        gvk,
		groupResource: schema.GroupResource{
			Group:    gvk.Group,
			Resource: resource,
		},
	}
}

func FromPodSecurityPolicyTemplateHandlerToHandler(sync PodSecurityPolicyTemplateHandler) generic.Handler {
	return func(key string, obj runtime.Object) (ret runtime.Object, err error) {
		var v *v3.PodSecurityPolicyTemplate
		if obj == nil {
			v, err = sync(key, nil)
		} else {
			v, err = sync(key, obj.(*v3.PodSecurityPolicyTemplate))
		}
		if v == nil {
			return nil, err
		}
		return v, err
	}
}

func (c *podSecurityPolicyTemplateController) Updater() generic.Updater {
	return func(obj runtime.Object) (runtime.Object, error) {
		newObj, err := c.Update(obj.(*v3.PodSecurityPolicyTemplate))
		if newObj == nil {
			return nil, err
		}
		return newObj, err
	}
}

func UpdatePodSecurityPolicyTemplateDeepCopyOnChange(client PodSecurityPolicyTemplateClient, obj *v3.PodSecurityPolicyTemplate, handler func(obj *v3.PodSecurityPolicyTemplate) (*v3.PodSecurityPolicyTemplate, error)) (*v3.PodSecurityPolicyTemplate, error) {
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

func (c *podSecurityPolicyTemplateController) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	c.controller.RegisterHandler(ctx, name, controller.SharedControllerHandlerFunc(handler))
}

func (c *podSecurityPolicyTemplateController) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), handler))
}

func (c *podSecurityPolicyTemplateController) OnChange(ctx context.Context, name string, sync PodSecurityPolicyTemplateHandler) {
	c.AddGenericHandler(ctx, name, FromPodSecurityPolicyTemplateHandlerToHandler(sync))
}

func (c *podSecurityPolicyTemplateController) OnRemove(ctx context.Context, name string, sync PodSecurityPolicyTemplateHandler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), FromPodSecurityPolicyTemplateHandlerToHandler(sync)))
}

func (c *podSecurityPolicyTemplateController) Enqueue(name string) {
	c.controller.Enqueue("", name)
}

func (c *podSecurityPolicyTemplateController) EnqueueAfter(name string, duration time.Duration) {
	c.controller.EnqueueAfter("", name, duration)
}

func (c *podSecurityPolicyTemplateController) Informer() cache.SharedIndexInformer {
	return c.controller.Informer()
}

func (c *podSecurityPolicyTemplateController) GroupVersionKind() schema.GroupVersionKind {
	return c.gvk
}

func (c *podSecurityPolicyTemplateController) Cache() PodSecurityPolicyTemplateCache {
	return &podSecurityPolicyTemplateCache{
		indexer:  c.Informer().GetIndexer(),
		resource: c.groupResource,
	}
}

func (c *podSecurityPolicyTemplateController) Create(obj *v3.PodSecurityPolicyTemplate) (*v3.PodSecurityPolicyTemplate, error) {
	result := &v3.PodSecurityPolicyTemplate{}
	return result, c.client.Create(context.TODO(), "", obj, result, metav1.CreateOptions{})
}

func (c *podSecurityPolicyTemplateController) Update(obj *v3.PodSecurityPolicyTemplate) (*v3.PodSecurityPolicyTemplate, error) {
	result := &v3.PodSecurityPolicyTemplate{}
	return result, c.client.Update(context.TODO(), "", obj, result, metav1.UpdateOptions{})
}

func (c *podSecurityPolicyTemplateController) Delete(name string, options *metav1.DeleteOptions) error {
	if options == nil {
		options = &metav1.DeleteOptions{}
	}
	return c.client.Delete(context.TODO(), "", name, *options)
}

func (c *podSecurityPolicyTemplateController) Get(name string, options metav1.GetOptions) (*v3.PodSecurityPolicyTemplate, error) {
	result := &v3.PodSecurityPolicyTemplate{}
	return result, c.client.Get(context.TODO(), "", name, result, options)
}

func (c *podSecurityPolicyTemplateController) List(opts metav1.ListOptions) (*v3.PodSecurityPolicyTemplateList, error) {
	result := &v3.PodSecurityPolicyTemplateList{}
	return result, c.client.List(context.TODO(), "", result, opts)
}

func (c *podSecurityPolicyTemplateController) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c.client.Watch(context.TODO(), "", opts)
}

func (c *podSecurityPolicyTemplateController) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (*v3.PodSecurityPolicyTemplate, error) {
	result := &v3.PodSecurityPolicyTemplate{}
	return result, c.client.Patch(context.TODO(), "", name, pt, data, result, metav1.PatchOptions{}, subresources...)
}

type podSecurityPolicyTemplateCache struct {
	indexer  cache.Indexer
	resource schema.GroupResource
}

func (c *podSecurityPolicyTemplateCache) Get(name string) (*v3.PodSecurityPolicyTemplate, error) {
	obj, exists, err := c.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(c.resource, name)
	}
	return obj.(*v3.PodSecurityPolicyTemplate), nil
}

func (c *podSecurityPolicyTemplateCache) List(selector labels.Selector) (ret []*v3.PodSecurityPolicyTemplate, err error) {

	err = cache.ListAll(c.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v3.PodSecurityPolicyTemplate))
	})

	return ret, err
}

func (c *podSecurityPolicyTemplateCache) AddIndexer(indexName string, indexer PodSecurityPolicyTemplateIndexer) {
	utilruntime.Must(c.indexer.AddIndexers(map[string]cache.IndexFunc{
		indexName: func(obj interface{}) (strings []string, e error) {
			return indexer(obj.(*v3.PodSecurityPolicyTemplate))
		},
	}))
}

func (c *podSecurityPolicyTemplateCache) GetByIndex(indexName, key string) (result []*v3.PodSecurityPolicyTemplate, err error) {
	objs, err := c.indexer.ByIndex(indexName, key)
	if err != nil {
		return nil, err
	}
	result = make([]*v3.PodSecurityPolicyTemplate, 0, len(objs))
	for _, obj := range objs {
		result = append(result, obj.(*v3.PodSecurityPolicyTemplate))
	}
	return result, nil
}
