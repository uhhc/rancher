package client

import (
	"github.com/rancher/wrangler/pkg/summary"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

type summaryClient struct {
	client dynamic.Interface
}

var _ Interface = &summaryClient{}

func NewForDynamicClient(client dynamic.Interface) Interface {
	return &summaryClient{client: client}
}

type summaryResourceClient struct {
	client    dynamic.Interface
	namespace string
	resource  schema.GroupVersionResource
}

func (c *summaryClient) Resource(resource schema.GroupVersionResource) NamespaceableResourceInterface {
	return &summaryResourceClient{client: c.client, resource: resource}
}

func (c *summaryResourceClient) Namespace(ns string) ResourceInterface {
	ret := *c
	ret.namespace = ns
	return &ret
}

func (c *summaryResourceClient) List(opts metav1.ListOptions) (*summary.SummarizedObjectList, error) {
	var (
		u   *unstructured.UnstructuredList
		err error
	)

	if c.namespace == "" {
		u, err = c.client.Resource(c.resource).List(opts)
	} else {
		u, err = c.client.Resource(c.resource).Namespace(c.namespace).List(opts)
	}
	if err != nil {
		return nil, err
	}

	list := &summary.SummarizedObjectList{
		TypeMeta: metav1.TypeMeta{
			Kind:       u.GetKind(),
			APIVersion: u.GetAPIVersion(),
		},
		ListMeta: metav1.ListMeta{
			ResourceVersion:    u.GetResourceVersion(),
			Continue:           u.GetContinue(),
			RemainingItemCount: u.GetRemainingItemCount(),
		},
	}

	for _, obj := range u.Items {
		list.Items = append(list.Items, *summary.Summarized(&obj))
	}

	return list, nil
}

func (c *summaryResourceClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	var (
		resp watch.Interface
		err  error
	)

	eventChan := make(chan watch.Event)

	if c.namespace == "" {
		resp, err = c.client.Resource(c.resource).Watch(opts)
	} else {
		resp, err = c.client.Resource(c.resource).Namespace(c.namespace).Watch(opts)
	}
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(eventChan)
		for event := range resp.ResultChan() {
			switch event.Type {
			case watch.Added:
				fallthrough
			case watch.Modified:
				fallthrough
			case watch.Deleted:
				event.Object = summary.Summarized(event.Object)
			default:
			}
			eventChan <- event
		}
	}()

	return &watcher{
		Interface: resp,
		eventChan: eventChan,
	}, nil
}

type watcher struct {
	watch.Interface
	eventChan chan watch.Event
}

func (w watcher) ResultChan() <-chan watch.Event {
	return w.eventChan
}
