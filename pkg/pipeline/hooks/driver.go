package hooks

import (
	"net/http"

	"github.com/uhhc/rancher/pkg/pipeline/hooks/drivers"
	"github.com/rancher/types/config"
)

var Drivers map[string]Driver

type Driver interface {
	Execute(req *http.Request) (int, error)
}

func RegisterDrivers(Management *config.ScaledContext) {
	projectLister := Management.Management.Projects("").Controller().Lister()
	pipelineLister := Management.Project.Pipelines("").Controller().Lister()
	pipelineExecutions := Management.Project.PipelineExecutions("")
	sourceCodeCredentials := Management.Project.SourceCodeCredentials("")
	sourceCodeCredentialLister := Management.Project.SourceCodeCredentials("").Controller().Lister()

	Drivers = map[string]Driver{}
	Drivers[drivers.GithubWebhookHeader] = drivers.GithubDriver{
		ProjectLister:				projectLister,
		PipelineLister:             pipelineLister,
		PipelineExecutions:         pipelineExecutions,
		SourceCodeCredentials:      sourceCodeCredentials,
		SourceCodeCredentialLister: sourceCodeCredentialLister,
	}
	Drivers[drivers.GitlabWebhookHeader] = drivers.GitlabDriver{
		ProjectLister:				projectLister,
		PipelineLister:             pipelineLister,
		PipelineExecutions:         pipelineExecutions,
		SourceCodeCredentials:      sourceCodeCredentials,
		SourceCodeCredentialLister: sourceCodeCredentialLister,
	}
	Drivers[drivers.BitbucketCloudWebhookHeader] = drivers.BitbucketCloudDriver{
		ProjectLister:				projectLister,
		PipelineLister:             pipelineLister,
		PipelineExecutions:         pipelineExecutions,
		SourceCodeCredentials:      sourceCodeCredentials,
		SourceCodeCredentialLister: sourceCodeCredentialLister,
	}
	Drivers[drivers.BitbucketServerWebhookHeader] = drivers.BitbucketServerDriver{
		ProjectLister:				projectLister,
		PipelineLister:             pipelineLister,
		PipelineExecutions:         pipelineExecutions,
		SourceCodeCredentials:      sourceCodeCredentials,
		SourceCodeCredentialLister: sourceCodeCredentialLister,
	}
}
