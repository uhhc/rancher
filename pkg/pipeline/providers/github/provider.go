package github

import (
	"fmt"
	"github.com/mitchellh/mapstructure"
	"github.com/rancher/norman/store/subtype"
	"github.com/rancher/norman/types"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/rancher/pkg/pipeline/remote/model"
	mv3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/apis/project.cattle.io/v3"
	"github.com/rancher/types/apis/project.cattle.io/v3/schema"
	"github.com/rancher/types/client/project/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

type GhProvider struct {
	SourceCodeProviderConfigs  v3.SourceCodeProviderConfigInterface
	SourceCodeCredentials      v3.SourceCodeCredentialInterface
	SourceCodeCredentialLister v3.SourceCodeCredentialLister
	SourceCodeRepositories     v3.SourceCodeRepositoryInterface
	Pipelines                  v3.PipelineInterface
	PipelineExecutions         v3.PipelineExecutionInterface

	PipelineIndexer             cache.Indexer
	PipelineExecutionIndexer    cache.Indexer
	SourceCodeCredentialIndexer cache.Indexer
	SourceCodeRepositoryIndexer cache.Indexer

	AuthConfigs mv3.AuthConfigInterface
}

func (g *GhProvider) CustomizeSchemas(schemas *types.Schemas) {
	scpConfigBaseSchema := schemas.Schema(&schema.Version, client.SourceCodeProviderConfigType)
	configSchema := schemas.Schema(&schema.Version, client.GithubPipelineConfigType)
	configSchema.ActionHandler = g.ActionHandler
	configSchema.Formatter = g.Formatter
	configSchema.Store = subtype.NewSubTypeStore(client.GithubPipelineConfigType, scpConfigBaseSchema.Store)

	providerBaseSchema := schemas.Schema(&schema.Version, client.SourceCodeProviderType)
	providerSchema := schemas.Schema(&schema.Version, client.GithubProviderType)
	providerSchema.Formatter = g.providerFormatter
	providerSchema.ActionHandler = g.providerActionHandler
	providerSchema.Store = subtype.NewSubTypeStore(client.GithubProviderType, providerBaseSchema.Store)
}

func (g *GhProvider) GetName() string {
	return model.GithubType
}

func (g *GhProvider) TransformToSourceCodeProvider(config map[string]interface{}) map[string]interface{} {
	p := transformToSourceCodeProvider(config)
	p[client.GithubProviderFieldRedirectURL] = formGithubRedirectURLFromMap(config)
	return p
}

func transformToSourceCodeProvider(config map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	if m, ok := config["metadata"].(map[string]interface{}); ok {
		result["id"] = fmt.Sprintf("%v:%v", m[client.ObjectMetaFieldNamespace], m[client.ObjectMetaFieldName])
	}
	if t := convert.ToString(config[client.SourceCodeProviderFieldType]); t != "" {
		result[client.SourceCodeProviderFieldType] = client.GithubProviderType
	}
	if t := convert.ToString(config[projectNameField]); t != "" {
		result["projectId"] = t
	}
	result[client.GithubProviderFieldRedirectURL] = formGithubRedirectURLFromMap(config)

	return result
}

func (g *GhProvider) GetProviderConfig(projectID string) (interface{}, error) {
	scpConfigObj, err := g.SourceCodeProviderConfigs.ObjectClient().UnstructuredClient().GetNamespaced(projectID, model.GithubType, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve GithubConfig, error: %v", err)
	}

	u, ok := scpConfigObj.(runtime.Unstructured)
	if !ok {
		return nil, fmt.Errorf("failed to retrieve GithubConfig, cannot read k8s Unstructured data")
	}
	storedGithubPipelineConfigMap := u.UnstructuredContent()

	storedGithubPipelineConfig := &v3.GithubPipelineConfig{}
	if err := mapstructure.Decode(storedGithubPipelineConfigMap, storedGithubPipelineConfig); err != nil {
		return nil, fmt.Errorf("failed to decode the config, error: %v", err)
	}

	metadataMap, ok := storedGithubPipelineConfigMap["metadata"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to retrieve GithubConfig metadata, cannot read k8s Unstructured data")
	}

	if storedGithubPipelineConfig.Inherit {
		globalConfig, err := g.getGithubConfigCR()
		if err != nil {
			return nil, err
		}
		storedGithubPipelineConfig.ClientSecret = globalConfig.ClientSecret
	}

	typemeta := &metav1.ObjectMeta{}
	//time.Time cannot decode directly
	delete(metadataMap, "creationTimestamp")
	if err := mapstructure.Decode(metadataMap, typemeta); err != nil {
		return nil, fmt.Errorf("failed to decode the config, error: %v", err)
	}
	storedGithubPipelineConfig.ObjectMeta = *typemeta
	storedGithubPipelineConfig.APIVersion = "project.cattle.io/v3"
	storedGithubPipelineConfig.Kind = v3.SourceCodeProviderConfigGroupVersionKind.Kind
	return storedGithubPipelineConfig, nil
}
