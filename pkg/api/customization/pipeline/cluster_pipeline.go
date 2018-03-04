package pipeline

import (
	"encoding/json"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rancher/norman/api/access"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/controllers/user/pipeline/remote"
	"github.com/rancher/rancher/pkg/controllers/user/pipeline/utils"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/client/management/v3"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"
	"strings"
)

type ClusterPipelineHandler struct {
	ClusterPipelines           v3.ClusterPipelineInterface
	ClusterPipelineLister      v3.ClusterPipelineLister
	SourceCodeCredentials      v3.SourceCodeCredentialInterface
	SourceCodeCredentialLister v3.SourceCodeCredentialLister
	SourceCodeRepositories     v3.SourceCodeRepositoryInterface
	SourceCodeRepositoryLister v3.SourceCodeRepositoryLister

	AuthConfigs v3.AuthConfigInterface
}

func ClusterPipelineFormatter(apiContext *types.APIContext, resource *types.RawResource) {
	resource.AddAction(apiContext, "deploy")
	resource.AddAction(apiContext, "destroy")
	resource.AddAction(apiContext, "revokeapp")
	resource.AddAction(apiContext, "authapp")
	resource.AddAction(apiContext, "authuser")
	resource.Links["envvars"] = apiContext.URLBuilder.Link("envvars", resource)
}

func (h *ClusterPipelineHandler) LinkHandler(apiContext *types.APIContext, next types.RequestHandler) error {
	if apiContext.Link == "envvars" {
		bytes, err := json.Marshal(utils.PreservedEnvVars)
		if err != nil {
			return err
		}
		apiContext.Response.Write(bytes)
		return nil
	}

	return httperror.NewAPIError(httperror.NotFound, "Link not found")
}

func (h *ClusterPipelineHandler) ActionHandler(actionName string, action *types.Action, apiContext *types.APIContext) error {

	logrus.Debugf("do cluster pipeline action:%s", actionName)
	switch actionName {
	case "deploy":
		return h.deploy(apiContext)
	case "destroy":
		return h.destroy(apiContext)
	case "revokeapp":
		return h.revokeapp(apiContext)
	case "authapp":
		return h.authapp(apiContext)
	case "authuser":
		return h.authuser(apiContext)
	}

	return httperror.NewAPIError(httperror.InvalidAction, "unsupported action")
}

func (h *ClusterPipelineHandler) deploy(apiContext *types.APIContext) error {
	parts := strings.Split(apiContext.ID, ":")
	if len(parts) <= 1 {
		return errors.New("invalid ID")
	}
	ns := parts[0]
	name := parts[1]
	clusterPipeline, err := h.ClusterPipelineLister.Get(ns, name)
	if err != nil {
		return err
	}
	if !clusterPipeline.Spec.Deploy {
		clusterPipeline.Spec.Deploy = true
		if _, err = h.ClusterPipelines.Update(clusterPipeline); err != nil {
			return err
		}
	}
	data := map[string]interface{}{}
	if err := access.ByID(apiContext, apiContext.Version, apiContext.Type, apiContext.ID, &data); err != nil {
		return err
	}
	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}

func (h *ClusterPipelineHandler) destroy(apiContext *types.APIContext) error {
	parts := strings.Split(apiContext.ID, ":")
	if len(parts) <= 1 {
		return errors.New("invalid ID")
	}
	ns := parts[0]
	name := parts[1]
	clusterPipeline, err := h.ClusterPipelineLister.Get(ns, name)
	if err != nil {
		return err
	}
	if clusterPipeline.Spec.Deploy {
		clusterPipeline.Spec.Deploy = false
		if _, err = h.ClusterPipelines.Update(clusterPipeline); err != nil {
			return err
		}
	}
	data := map[string]interface{}{}
	if err := access.ByID(apiContext, apiContext.Version, apiContext.Type, apiContext.ID, &data); err != nil {
		return err
	}
	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}

func (h *ClusterPipelineHandler) authapp(apiContext *types.APIContext) error {

	parts := strings.Split(apiContext.ID, ":")
	if len(parts) <= 1 {
		return errors.New("invalid ID")
	}
	ns := parts[0]
	name := parts[1]

	authAppInput := v3.AuthAppInput{}
	requestBytes, err := ioutil.ReadAll(apiContext.Request.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(requestBytes, &authAppInput); err != nil {
		return err
	}
	clusterPipeline, err := h.ClusterPipelineLister.Get(ns, name)
	if err != nil {
		return err
	}

	if authAppInput.SourceCodeType == "github" {
		clusterPipeline.Spec.GithubConfig = &v3.GithubClusterConfig{
			TLS:          authAppInput.TLS,
			Host:         authAppInput.Host,
			ClientID:     authAppInput.ClientID,
			ClientSecret: authAppInput.ClientSecret,
			RedirectURL:  authAppInput.RedirectURL,
		}
		if authAppInput.InheritGlobal {
			globalConfig, err := h.getGithubConfigCR()
			if err != nil {
				return err
			}
			clusterPipeline.Spec.GithubConfig.TLS = globalConfig.TLS
			clusterPipeline.Spec.GithubConfig.Host = globalConfig.Hostname
			clusterPipeline.Spec.GithubConfig.ClientID = globalConfig.ClientID
			clusterPipeline.Spec.GithubConfig.ClientSecret = globalConfig.ClientSecret
		}
	} else {
		return fmt.Errorf("Error unsupported source code type %s", authAppInput.SourceCodeType)
	}
	//oauth and add user
	userName := apiContext.Request.Header.Get("Impersonate-User")
	sourceCodeCredential, err := h.authAddAccount(clusterPipeline, authAppInput.SourceCodeType, userName, authAppInput.RedirectURL, authAppInput.Code)
	if err != nil {
		return err
	}
	//update cluster pipeline config
	if _, err := h.ClusterPipelines.Update(clusterPipeline); err != nil {
		return err
	}

	data := map[string]interface{}{}
	if err := access.ByID(apiContext, apiContext.Version, apiContext.Type, apiContext.ID, &data); err != nil {
		return err
	}

	go refreshReposByCredential(h.SourceCodeRepositories, h.SourceCodeRepositoryLister, sourceCodeCredential)

	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}

func (h *ClusterPipelineHandler) authuser(apiContext *types.APIContext) error {

	parts := strings.Split(apiContext.ID, ":")
	if len(parts) <= 1 {
		return errors.New("invalid ID")
	}
	ns := parts[0]
	name := parts[1]

	authUserInput := v3.AuthUserInput{}
	requestBytes, err := ioutil.ReadAll(apiContext.Request.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(requestBytes, &authUserInput); err != nil {
		return err
	}

	clusterPipeline, err := h.ClusterPipelineLister.Get(ns, name)
	if err != nil {
		return err
	}

	if authUserInput.SourceCodeType == "github" && clusterPipeline.Spec.GithubConfig == nil {
		return errors.New("github oauth app is not configured")
	}

	//oauth and add user
	userName := apiContext.Request.Header.Get("Impersonate-User")
	logrus.Debugf("try auth with %v,%v,%v,%v,%v", clusterPipeline, authUserInput.SourceCodeType, userName, authUserInput.RedirectURL, authUserInput.Code)
	account, err := h.authAddAccount(clusterPipeline, authUserInput.SourceCodeType, userName, authUserInput.RedirectURL, authUserInput.Code)
	if err != nil {
		return err
	}
	data := map[string]interface{}{}
	if err := access.ByID(apiContext, apiContext.Version, client.SourceCodeCredentialType, account.Name, &data); err != nil {
		return err
	}

	go refreshReposByCredential(h.SourceCodeRepositories, h.SourceCodeRepositoryLister, account)

	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}

func (h *ClusterPipelineHandler) revokeapp(apiContext *types.APIContext) error {

	parts := strings.Split(apiContext.ID, ":")
	if len(parts) <= 1 {
		return errors.New("invalid ID")
	}
	ns := parts[0]
	name := parts[1]

	clusterPipeline, err := h.ClusterPipelineLister.Get(ns, name)
	if err != nil {
		return err
	}

	clusterPipeline.Spec.GithubConfig = nil
	_, err = h.ClusterPipelines.Update(clusterPipeline)
	if err != nil {
		return err
	}

	data := map[string]interface{}{}
	if err := access.ByID(apiContext, apiContext.Version, apiContext.Type, apiContext.ID, &data); err != nil {
		return err
	}

	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}

func (h *ClusterPipelineHandler) authAddAccount(clusterPipeline *v3.ClusterPipeline, remoteType string, userID string, redirectURL string, code string) (*v3.SourceCodeCredential, error) {

	if userID == "" {
		return nil, errors.New("unauth")
	}

	remote, err := remote.New(*clusterPipeline, remoteType)
	if err != nil {
		return nil, err
	}
	account, err := remote.Login(redirectURL, code)
	if err != nil {
		return nil, err
	}
	account.Name = fmt.Sprintf("%s-%s-%s", clusterPipeline.Spec.ClusterName, remoteType, account.Spec.LoginName)
	account.Spec.UserName = userID
	account.Spec.ClusterName = clusterPipeline.Spec.ClusterName
	account, err = h.SourceCodeCredentials.Create(account)
	if err != nil {
		return nil, err
	}
	return account, nil
}

func (h *ClusterPipelineHandler) getGithubConfigCR() (*v3.GithubConfig, error) {
	authConfigObj, err := h.AuthConfigs.ObjectClient().UnstructuredClient().Get("github", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve GithubConfig, error: %v", err)
	}

	u, ok := authConfigObj.(runtime.Unstructured)
	if !ok {
		return nil, fmt.Errorf("failed to retrieve GithubConfig, cannot read k8s Unstructured data")
	}
	storedGithubConfigMap := u.UnstructuredContent()

	storedGithubConfig := &v3.GithubConfig{}
	mapstructure.Decode(storedGithubConfigMap, storedGithubConfig)

	metadataMap, ok := storedGithubConfigMap["metadata"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to retrieve GithubConfig metadata, cannot read k8s Unstructured data")
	}

	typemeta := &metav1.ObjectMeta{}
	mapstructure.Decode(metadataMap, typemeta)
	storedGithubConfig.ObjectMeta = *typemeta

	return storedGithubConfig, nil
}
