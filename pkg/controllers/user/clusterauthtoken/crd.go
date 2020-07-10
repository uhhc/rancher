package clusterauthtoken

import (
	"context"

	"github.com/rancher/norman/store/crd"
	clusterSchema "github.com/rancher/rancher/pkg/types/apis/cluster.cattle.io/v3/schema"
	client "github.com/rancher/rancher/pkg/types/client/cluster/v3"
	"github.com/rancher/rancher/pkg/types/config"
)

func CRDSetup(ctx context.Context, apiContext *config.UserOnlyContext) error {
	factory, err := crd.NewFactoryFromClient(apiContext.RESTConfig)
	if err != nil {
		return err
	}
	factory.BatchCreateCRDs(ctx, config.UserStorageContext, apiContext.Schemas, &clusterSchema.Version,
		client.ClusterAuthTokenType,
		client.ClusterUserAttributeType,
	)
	return factory.BatchWait()
}
