package k8sproxy

import (
	"net/http"

	"github.com/uhhc/rancher/pkg/clusterrouter"
	"github.com/uhhc/rancher/pkg/k8slookup"
	"github.com/rancher/types/config"
	"github.com/rancher/types/config/dialer"
)

func New(scaledContext *config.ScaledContext, dialer dialer.Factory) http.Handler {
	return clusterrouter.New(&scaledContext.RESTConfig, k8slookup.New(scaledContext, true), dialer,
		scaledContext.Management.Clusters("").Controller().Lister())
}
