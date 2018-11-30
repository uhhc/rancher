package alert

import (
	"encoding/json"
	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/notifiers"
	"github.com/rancher/rancher/pkg/ref"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const testSMTPTitle = "Alert From Rancher: SMTP configuration validated"

func NotifierCollectionFormatter(apiContext *types.APIContext, collection *types.GenericCollection) {
	collection.AddAction(apiContext, "send")
}

func NotifierFormatter(apiContext *types.APIContext, resource *types.RawResource) {
	resource.AddAction(apiContext, "send")
}

func (h *Handler) NotifierActionHandler(actionName string, action *types.Action, apiContext *types.APIContext) error {
	switch actionName {
	case "send":
		return h.testNotifier(actionName, action, apiContext)
	}

	return httperror.NewAPIError(httperror.InvalidAction, "invalid action: "+actionName)

}

func (h *Handler) testNotifier(actionName string, action *types.Action, apiContext *types.APIContext) error {
	data, err := ioutil.ReadAll(apiContext.Request.Body)
	if err != nil {
		return errors.Wrap(err, "reading request body error")
	}
	input := &struct {
		Message string
		v3.NotifierSpec
	}{}
	if err = json.Unmarshal(data, input); err != nil {
		return errors.Wrap(err, "unmarshaling input error")
	}
	notifier := &v3.Notifier{
		Spec: input.NotifierSpec,
	}
	msg := input.Message
	if apiContext.ID != "" {
		ns, id := ref.Parse(apiContext.ID)
		notifier, err = h.Notifiers.GetNamespaced(ns, id, metav1.GetOptions{})
		if err != nil {
			return err
		}
	}
	notifierMessage := &notifiers.Message{
		Content: msg,
	}
	if notifier.Spec.SMTPConfig != nil {
		notifierMessage.Title = testSMTPTitle
	}
	return notifiers.SendMessage(notifier, "", notifierMessage)
}
