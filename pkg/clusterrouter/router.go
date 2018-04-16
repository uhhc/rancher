package clusterrouter

import (
	"encoding/json"
	"net/http"

	"github.com/rancher/norman/httperror"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config/dialer"
	"k8s.io/client-go/rest"
)

type Router struct {
	clusterLookup ClusterLookup
	serverFactory *factory
}

func New(localConfig *rest.Config, lookup ClusterLookup, dialer dialer.Factory, clusterLister v3.ClusterLister) http.Handler {
	serverFactory := newFactory(localConfig, dialer, lookup, clusterLister)
	return &Router{
		serverFactory: serverFactory,
	}
}

func (r *Router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	c, handler, err := r.serverFactory.get(req)
	if err != nil {
		response(rw, httperror.ServerError, err.Error())
		return
	}

	if c == nil {
		response(rw, httperror.NotFound, "No cluster available")
		return
	}

	handler.ServeHTTP(rw, req)
}

func response(rw http.ResponseWriter, code httperror.ErrorCode, message string) {
	rw.WriteHeader(code.Status)
	rw.Header().Set("content-type", "application/json")
	json.NewEncoder(rw).Encode(httperror.NewAPIError(code, message))
}
