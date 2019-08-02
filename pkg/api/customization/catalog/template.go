package catalog

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/rancher/norman/api/access"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/templatecontent"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	managementschema "github.com/rancher/types/apis/management.cattle.io/v3/schema"
	client "github.com/rancher/types/client/management/v3"
)

func TemplateFormatter(apiContext *types.APIContext, resource *types.RawResource) {
	// version links
	resource.Values["versionLinks"] = extractVersionLinks(apiContext, resource)

	//icon
	ic, ok := resource.Values["iconFilename"]
	if ok {
		if strings.HasPrefix(ic.(string), "file:") {
			delete(resource.Values, "icon")
			resource.Links["icon"] = apiContext.URLBuilder.Link("icon", resource)

		} else {
			delete(resource.Values, "icon")
			resource.Links["icon"] = ic.(string)
		}
	} else {
		delete(resource.Values, "icon")
		resource.Links["icon"] = apiContext.URLBuilder.Link("icon", resource)
	}

	//catalog link
	catalogSchema := apiContext.Schemas.Schema(&managementschema.Version, client.CatalogType)
	catalogName := strings.Split(resource.ID, "-")[0]
	resource.Links["catalog"] = apiContext.URLBuilder.ResourceLinkByID(catalogSchema, catalogName)

	// delete category
	delete(resource.Values, "category")

	// delete versions
	delete(resource.Values, "versions")
}

type TemplateWrapper struct {
	TemplateContentClient v3.TemplateContentInterface
}

func (t TemplateWrapper) TemplateIconHandler(apiContext *types.APIContext, next types.RequestHandler) error {
	switch apiContext.Link {
	case "icon":
		template := &client.Template{}
		if err := access.ByID(apiContext, apiContext.Version, apiContext.Type, apiContext.ID, template); err != nil {
			return err
		}
		if strings.HasPrefix(template.IconFilename, "http:") || strings.HasPrefix(template.IconFilename, "https:") {
			http.Error(apiContext.Response, "", http.StatusNoContent)
			return nil
		}

		data, err := templatecontent.GetTemplateFromTag(template.Icon, t.TemplateContentClient)
		if err != nil {
			return err
		}
		t, err := time.Parse(time.RFC3339, template.Created)
		if err != nil {
			return err
		}
		value, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return err
		}
		iconReader := bytes.NewReader(value)
		apiContext.Response.Header().Set("Cache-Control", "private, max-age=604800")
		// add security headers (similar to raw.githubusercontent)
		apiContext.Response.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		apiContext.Response.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(apiContext.Response, apiContext.Request, template.IconFilename, t, iconReader)
		return nil
	default:
		return httperror.NewAPIError(httperror.NotFound, "not found")
	}
}
