package k8sproxy

import (
	"net/http"

	"github.com/rancher/rancher/pkg/clusterrouter"
	"github.com/rancher/rancher/pkg/k8slookup"
	"github.com/rancher/rancher/pkg/types/config"
	"github.com/rancher/rancher/pkg/types/config/dialer"
)

func New(scaledContext *config.ScaledContext, dialer dialer.Factory) http.Handler {
	return clusterrouter.New(&scaledContext.RESTConfig, k8slookup.New(scaledContext, true), dialer,
		scaledContext.Management.Clusters("").Controller().Lister())
}
