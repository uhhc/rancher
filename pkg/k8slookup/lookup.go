package k8slookup

import (
	"net/http"

	"github.com/rancher/norman/api/access"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	client "github.com/rancher/rancher/pkg/client/generated/management/v3"
	"github.com/rancher/rancher/pkg/clusterrouter"
	v3 "github.com/rancher/rancher/pkg/generated/norman/management.cattle.io/v3"
	schema "github.com/rancher/rancher/pkg/schemas/management.cattle.io/v3"
	"github.com/rancher/rancher/pkg/types/config"
)

func New(context *config.ScaledContext, validate bool) clusterrouter.ClusterLookup {
	return &lookup{
		clusterLister: context.Management.Clusters("").Controller().Lister(),
		schemas:       context.Schemas,
		validate:      validate,
	}
}

type lookup struct {
	clusterLister v3.ClusterLister
	schemas       *types.Schemas
	validate      bool
}

func (l *lookup) Lookup(req *http.Request) (*v3.Cluster, error) {
	apiContext := types.NewAPIContext(req, nil, l.schemas)
	clusterID := clusterrouter.GetClusterID(req)
	if clusterID == "" {
		return nil, httperror.NewAPIError(httperror.NotFound, "failed to find cluster")
	}

	if l.validate {
		// check access
		if err := access.ByID(apiContext, &schema.Version, client.ClusterType, clusterID, &client.Cluster{}); err != nil {
			return nil, err
		}
	}

	return l.clusterLister.Get("", clusterID)
}
