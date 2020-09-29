package management

import (
	"context"

	"github.com/uhhc/rancher/pkg/clustermanager"
	"github.com/uhhc/rancher/pkg/controllers/management/k3supgrade"
	"github.com/uhhc/rancher/pkg/wrangler"
	"github.com/rancher/types/config"
)

func RegisterWrangler(ctx context.Context, wranglerContext *wrangler.Context, management *config.ManagementContext, manager *clustermanager.Manager) {
	// Add controllers to register here

	k3supgrade.Register(ctx, wranglerContext, management, manager)

}
