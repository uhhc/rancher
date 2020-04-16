/*
Copyright The Kubernetes Authors.

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

package v1

import (
	"context"
	"time"

	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/pkg/generic"
	v1 "k8s.io/api/core/v1"
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

type SecretHandler func(string, *v1.Secret) (*v1.Secret, error)

type SecretController interface {
	generic.ControllerMeta
	SecretClient

	OnChange(ctx context.Context, name string, sync SecretHandler)
	OnRemove(ctx context.Context, name string, sync SecretHandler)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, duration time.Duration)

	Cache() SecretCache
}

type SecretClient interface {
	Create(*v1.Secret) (*v1.Secret, error)
	Update(*v1.Secret) (*v1.Secret, error)

	Delete(namespace, name string, options *metav1.DeleteOptions) error
	Get(namespace, name string, options metav1.GetOptions) (*v1.Secret, error)
	List(namespace string, opts metav1.ListOptions) (*v1.SecretList, error)
	Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error)
	Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.Secret, err error)
}

type SecretCache interface {
	Get(namespace, name string) (*v1.Secret, error)
	List(namespace string, selector labels.Selector) ([]*v1.Secret, error)

	AddIndexer(indexName string, indexer SecretIndexer)
	GetByIndex(indexName, key string) ([]*v1.Secret, error)
}

type SecretIndexer func(obj *v1.Secret) ([]string, error)

type secretController struct {
	controller    controller.SharedController
	client        *client.Client
	gvk           schema.GroupVersionKind
	groupResource schema.GroupResource
}

func NewSecretController(gvk schema.GroupVersionKind, resource string, controller controller.SharedControllerFactory) SecretController {
	c, err := controller.ForKind(gvk)
	utilruntime.Must(err)
	return &secretController{
		controller: c,
		client:     c.Client(),
		gvk:        gvk,
		groupResource: schema.GroupResource{
			Group:    gvk.Group,
			Resource: resource,
		},
	}
}

func FromSecretHandlerToHandler(sync SecretHandler) generic.Handler {
	return func(key string, obj runtime.Object) (ret runtime.Object, err error) {
		var v *v1.Secret
		if obj == nil {
			v, err = sync(key, nil)
		} else {
			v, err = sync(key, obj.(*v1.Secret))
		}
		if v == nil {
			return nil, err
		}
		return v, err
	}
}

func (c *secretController) Updater() generic.Updater {
	return func(obj runtime.Object) (runtime.Object, error) {
		newObj, err := c.Update(obj.(*v1.Secret))
		if newObj == nil {
			return nil, err
		}
		return newObj, err
	}
}

func UpdateSecretDeepCopyOnChange(client SecretClient, obj *v1.Secret, handler func(obj *v1.Secret) (*v1.Secret, error)) (*v1.Secret, error) {
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

func (c *secretController) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	c.controller.RegisterHandler(ctx, name, controller.SharedControllerHandlerFunc(handler))
}

func (c *secretController) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), handler))
}

func (c *secretController) OnChange(ctx context.Context, name string, sync SecretHandler) {
	c.AddGenericHandler(ctx, name, FromSecretHandlerToHandler(sync))
}

func (c *secretController) OnRemove(ctx context.Context, name string, sync SecretHandler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), FromSecretHandlerToHandler(sync)))
}

func (c *secretController) Enqueue(namespace, name string) {
	c.controller.Enqueue(namespace, name)
}

func (c *secretController) EnqueueAfter(namespace, name string, duration time.Duration) {
	c.controller.EnqueueAfter(namespace, name, duration)
}

func (c *secretController) Informer() cache.SharedIndexInformer {
	return c.controller.Informer()
}

func (c *secretController) GroupVersionKind() schema.GroupVersionKind {
	return c.gvk
}

func (c *secretController) Cache() SecretCache {
	return &secretCache{
		indexer:  c.Informer().GetIndexer(),
		resource: c.groupResource,
	}
}

func (c *secretController) Create(obj *v1.Secret) (*v1.Secret, error) {
	result := &v1.Secret{}
	return result, c.client.Create(context.TODO(), obj.Namespace, obj, result, metav1.CreateOptions{})
}

func (c *secretController) Update(obj *v1.Secret) (*v1.Secret, error) {
	result := &v1.Secret{}
	return result, c.client.Update(context.TODO(), obj.Namespace, obj, result, metav1.UpdateOptions{})
}

func (c *secretController) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	if options == nil {
		options = &metav1.DeleteOptions{}
	}
	return c.client.Delete(context.TODO(), namespace, name, *options)
}

func (c *secretController) Get(namespace, name string, options metav1.GetOptions) (*v1.Secret, error) {
	result := &v1.Secret{}
	return result, c.client.Get(context.TODO(), namespace, name, result, options)
}

func (c *secretController) List(namespace string, opts metav1.ListOptions) (*v1.SecretList, error) {
	result := &v1.SecretList{}
	return result, c.client.List(context.TODO(), namespace, result, opts)
}

func (c *secretController) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c.client.Watch(context.TODO(), namespace, opts)
}

func (c *secretController) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (*v1.Secret, error) {
	result := &v1.Secret{}
	return result, c.client.Patch(context.TODO(), namespace, name, pt, data, result, metav1.PatchOptions{}, subresources...)
}

type secretCache struct {
	indexer  cache.Indexer
	resource schema.GroupResource
}

func (c *secretCache) Get(namespace, name string) (*v1.Secret, error) {
	obj, exists, err := c.indexer.GetByKey(namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(c.resource, name)
	}
	return obj.(*v1.Secret), nil
}

func (c *secretCache) List(namespace string, selector labels.Selector) (ret []*v1.Secret, err error) {

	err = cache.ListAllByNamespace(c.indexer, namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.Secret))
	})

	return ret, err
}

func (c *secretCache) AddIndexer(indexName string, indexer SecretIndexer) {
	utilruntime.Must(c.indexer.AddIndexers(map[string]cache.IndexFunc{
		indexName: func(obj interface{}) (strings []string, e error) {
			return indexer(obj.(*v1.Secret))
		},
	}))
}

func (c *secretCache) GetByIndex(indexName, key string) (result []*v1.Secret, err error) {
	objs, err := c.indexer.ByIndex(indexName, key)
	if err != nil {
		return nil, err
	}
	result = make([]*v1.Secret, 0, len(objs))
	for _, obj := range objs {
		result = append(result, obj.(*v1.Secret))
	}
	return result, nil
}
