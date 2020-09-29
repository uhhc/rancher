package remote

import (
	"errors"

	"github.com/uhhc/rancher/pkg/pipeline/remote/bitbucketcloud"
	"github.com/uhhc/rancher/pkg/pipeline/remote/bitbucketserver"
	"github.com/uhhc/rancher/pkg/pipeline/remote/github"
	"github.com/uhhc/rancher/pkg/pipeline/remote/gitlab"
	"github.com/uhhc/rancher/pkg/pipeline/remote/model"
	v3 "github.com/rancher/types/apis/project.cattle.io/v3"
)

func New(config interface{}) (model.Remote, error) {
	if config == nil {
		return github.New(nil)
	}
	switch config := config.(type) {
	case *v3.GithubPipelineConfig:
		return github.New(config)
	case *v3.GitlabPipelineConfig:
		return gitlab.New(config)
	case *v3.BitbucketCloudPipelineConfig:
		return bitbucketcloud.New(config)
	case *v3.BitbucketServerPipelineConfig:
		return bitbucketserver.New(config)
	}

	return nil, errors.New("unsupported remote type")
}
