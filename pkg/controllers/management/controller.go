package management

import (
	"context"

	"github.com/uhhc/rancher/pkg/clustermanager"
	"github.com/uhhc/rancher/pkg/controllers/management/auth"
	"github.com/uhhc/rancher/pkg/controllers/management/catalog"
	"github.com/uhhc/rancher/pkg/controllers/management/certsexpiration"
	"github.com/uhhc/rancher/pkg/controllers/management/cis"
	"github.com/uhhc/rancher/pkg/controllers/management/cloudcredential"
	"github.com/uhhc/rancher/pkg/controllers/management/cluster"
	"github.com/uhhc/rancher/pkg/controllers/management/clusterdeploy"
	"github.com/uhhc/rancher/pkg/controllers/management/clustergc"
	"github.com/uhhc/rancher/pkg/controllers/management/clusterprovisioner"
	"github.com/uhhc/rancher/pkg/controllers/management/clusterstats"
	"github.com/uhhc/rancher/pkg/controllers/management/clusterstatus"
	"github.com/uhhc/rancher/pkg/controllers/management/clustertemplate"
	"github.com/uhhc/rancher/pkg/controllers/management/compose"
	"github.com/uhhc/rancher/pkg/controllers/management/drivers/kontainerdriver"
	"github.com/uhhc/rancher/pkg/controllers/management/drivers/nodedriver"
	"github.com/uhhc/rancher/pkg/controllers/management/etcdbackup"
	"github.com/uhhc/rancher/pkg/controllers/management/globaldns"
	"github.com/uhhc/rancher/pkg/controllers/management/kontainerdrivermetadata"
	"github.com/uhhc/rancher/pkg/controllers/management/multiclusterapp"
	"github.com/uhhc/rancher/pkg/controllers/management/node"
	"github.com/uhhc/rancher/pkg/controllers/management/nodepool"
	"github.com/uhhc/rancher/pkg/controllers/management/nodetemplate"
	"github.com/uhhc/rancher/pkg/controllers/management/podsecuritypolicy"
	"github.com/uhhc/rancher/pkg/controllers/management/rkeworkerupgrader"
	"github.com/uhhc/rancher/pkg/controllers/management/usercontrollers"
	"github.com/rancher/types/config"
)

func Register(ctx context.Context, management *config.ManagementContext, manager *clustermanager.Manager) {
	// auth handlers need to run early to create namespaces that back clusters and projects
	// also, these handlers are purely in the mgmt plane, so they are lightweight compared to those that interact with machines and clusters
	auth.RegisterEarly(ctx, management, manager)
	usercontrollers.RegisterEarly(ctx, management, manager)

	// a-z
	catalog.Register(ctx, management)
	certsexpiration.Register(ctx, management)
	cluster.Register(ctx, management)
	clusterdeploy.Register(ctx, management, manager)
	clustergc.Register(ctx, management)
	clusterprovisioner.Register(ctx, management)
	clusterstats.Register(ctx, management, manager)
	clusterstatus.Register(ctx, management)
	compose.Register(ctx, management, manager)
	kontainerdriver.Register(ctx, management)
	kontainerdrivermetadata.Register(ctx, management)
	nodedriver.Register(ctx, management)
	nodepool.Register(ctx, management)
	cloudcredential.Register(ctx, management)
	node.Register(ctx, management)
	podsecuritypolicy.Register(ctx, management)
	etcdbackup.Register(ctx, management)
	cis.Register(ctx, management)
	globaldns.Register(ctx, management)
	multiclusterapp.Register(ctx, management, manager)
	clustertemplate.Register(ctx, management)
	nodetemplate.Register(ctx, management)
	rkeworkerupgrader.Register(ctx, management, manager.ScaledContext)

	// Register last
	auth.RegisterLate(ctx, management)

	// Ensure caches are available for user controllers, these are used as part of
	// registration
	management.Management.ClusterAlertGroups("").Controller()
	management.Management.ClusterAlertRules("").Controller()

}
