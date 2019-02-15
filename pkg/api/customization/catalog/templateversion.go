package catalog

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rancher/norman/api/access"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	helmlib "github.com/rancher/rancher/pkg/catalog/helm"
	"github.com/rancher/rancher/pkg/controllers/user/helm/common"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	managementschema "github.com/rancher/types/apis/management.cattle.io/v3/schema"
	"github.com/rancher/types/client/management/v3"
	"github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

type TemplateVerionFormatterWrapper struct {
	CatalogLister        v3.CatalogLister
	ClusterCatalogLister v3.ClusterCatalogLister
	ProjectCatalogLister v3.ProjectCatalogLister
}

var supportedFiles = []string{"catalog.yml", "catalog.yaml", "questions.yml", "questions.yaml"}

type catalogYml struct {
	Questions []v3.Question `yaml:"questions,omitempty"`
}

func (t TemplateVerionFormatterWrapper) TemplateVersionFormatter(apiContext *types.APIContext, resource *types.RawResource) {
	delete(resource.Values, "files")
	delete(resource.Values, "readme")
	delete(resource.Values, "appReadme")

	resource.Links["readme"] = apiContext.URLBuilder.Link("readme", resource)

	version := resource.Values["version"].(string)
	if revision, ok := resource.Values["revision"]; ok {
		version = strconv.FormatInt(revision.(int64), 10)
	}
	templateID := strings.TrimSuffix(resource.ID, "-"+version)
	templateSchema := apiContext.Schemas.Schema(&managementschema.Version, client.TemplateType)
	resource.Links["template"] = apiContext.URLBuilder.ResourceLinkByID(templateSchema, templateID)

	upgradeLinks, ok := resource.Values["upgradeVersionLinks"].(map[string]interface{})
	if ok {
		linkMap := map[string]string{}
		templateVersionSchema := apiContext.Schemas.Schema(&managementschema.Version, client.TemplateVersionType)
		for v, versionID := range upgradeLinks {
			linkMap[v] = apiContext.URLBuilder.ResourceLinkByID(templateVersionSchema, versionID.(string))
		}
		delete(resource.Values, "upgradeVersionLinks")
		resource.Values["upgradeVersionLinks"] = linkMap
	}

	externalID, ok := resource.Values["externalId"].(string)
	if !ok {
		logrus.Errorf("TemplateVersion has no external ID: %s", resource.ID)
		return
	}
	versionName, ok := resource.Values["versionName"].(string)
	if !ok {
		logrus.Errorf("TemplateVersion has no version Name: %s", resource.ID)
		return
	}
	versionDir, _ := resource.Values["versionDir"].(string)
	versionURLsInterface, _ := resource.Values["versionUrls"].([]interface{})
	versionURLs := make([]string, len(versionURLsInterface))
	for i, url := range versionURLsInterface {
		versionURLs[i], _ = url.(string)
	}

	files, err := t.loadChart(&client.CatalogTemplateVersion{
		ExternalID:  externalID,
		Version:     version,
		VersionName: versionName,
		VersionDir:  versionDir,
		VersionURLs: versionURLs,
	}, nil)
	if err != nil {
		logrus.Errorf("Failed to load chart: %s", err)
		return
	}

	for name, content := range files {
		if strings.EqualFold(fmt.Sprintf("%s/%s", versionName, "app-readme.md"), name) {
			resource.Links["app-readme"] = apiContext.URLBuilder.Link("app-readme", resource)
		}
		for _, f := range supportedFiles {
			if strings.EqualFold(fmt.Sprintf("%s/%s", versionName, f), name) {
				var value catalogYml
				if err := yaml.Unmarshal([]byte(content), &value); err != nil {
					logrus.Errorf("Failed to load file %s : %s", f, err)
				}
				if len(value.Questions) > 0 {
					resource.Values["questions"] = value.Questions
				}
			}
		}
		files[name] = base64.StdEncoding.EncodeToString([]byte(content))
	}
	resource.Values["files"] = files
}

func extractVersionLinks(apiContext *types.APIContext, resource *types.RawResource) map[string]string {
	schema := apiContext.Schemas.Schema(&managementschema.Version, client.TemplateVersionType)
	r := map[string]string{}
	versionMap, ok := resource.Values["versions"].([]interface{})
	if ok {
		for _, version := range versionMap {
			revision := ""
			if v, ok := version.(map[string]interface{})["revision"].(int64); ok {
				revision = strconv.FormatInt(v, 10)
			}
			version := version.(map[string]interface{})["version"].(string)
			versionID := fmt.Sprintf("%v-%v", resource.ID, version)
			if revision != "" {
				versionID = fmt.Sprintf("%v-%v", resource.ID, revision)
			}
			r[version] = apiContext.URLBuilder.ResourceLinkByID(schema, versionID)
		}
	}
	return r
}

func (t TemplateVerionFormatterWrapper) TemplateVersionReadmeHandler(apiContext *types.APIContext, next types.RequestHandler) error {
	templateVersion := &client.CatalogTemplateVersion{}
	if err := access.ByID(apiContext, apiContext.Version, apiContext.Type, apiContext.ID, templateVersion); err != nil {
		return err
	}

	var filter []string
	switch apiContext.Link {
	case "readme":
		filter = []string{templateVersion.VersionName + "/readme.md"}
	case "app-readme":
		filter = []string{templateVersion.VersionName + "/app-readme.md"}
	default:
		return httperror.NewAPIError(httperror.NotFound, "not found")
	}
	files, err := t.loadChart(templateVersion, filter)
	if err != nil {
		return err
	}
	return sendFile(templateVersion, files, apiContext)
}

func (t TemplateVerionFormatterWrapper) loadChart(templateVersion *client.CatalogTemplateVersion, filter []string) (map[string]string, error) {
	namespace, catalogName, catalogType, _, _, err := common.SplitExternalID(templateVersion.ExternalID)
	catalog, err := helmlib.GetCatalog(catalogType, namespace, catalogName, t.CatalogLister, t.ClusterCatalogLister, t.ProjectCatalogLister)
	if err != nil {
		return nil, err
	}

	helm, err := helmlib.New(catalog)
	if err != nil {
		return nil, err
	}

	return helm.LoadChart(&v3.TemplateVersionSpec{
		Version:     templateVersion.Version,
		VersionName: templateVersion.VersionName,
		VersionDir:  templateVersion.VersionDir,
		VersionURLs: templateVersion.VersionURLs,
	}, filter)
}

func sendFile(templateVersion *client.CatalogTemplateVersion, files map[string]string, apiContext *types.APIContext) error {
	var (
		fileContents string
		err          error
	)
	for name, content := range files {
		if strings.EqualFold(fmt.Sprintf("%s/%s.md", templateVersion.VersionName, apiContext.Link), name) {
			fileContents = content
		}
	}
	reader := bytes.NewReader([]byte(fileContents))
	t, err := time.Parse(time.RFC3339, templateVersion.Created)
	if err != nil {
		return err
	}
	apiContext.Response.Header().Set("Content-Type", "text/plain")
	http.ServeContent(apiContext.Response, apiContext.Request, apiContext.Link, t, reader)
	return nil
}
