package pipeline

import (
	"context"

	"github.com/uhhc/rancher/pkg/controllers/user/pipeline/controller/pipeline"
	"github.com/uhhc/rancher/pkg/controllers/user/pipeline/controller/pipelineexecution"
	"github.com/uhhc/rancher/pkg/controllers/user/pipeline/controller/project"
	"github.com/rancher/types/config"
)

func Register(ctx context.Context, cluster *config.UserContext) {
	pipeline.Register(ctx, cluster)
	pipelineexecution.Register(ctx, cluster)
	project.Register(ctx, cluster)
}
