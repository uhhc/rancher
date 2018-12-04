package catalog

import (
	"github.com/rancher/rancher/pkg/ref"
	"time"

	"bytes"
	"encoding/json"
	"net/http"

	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/ghodss/yaml"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/client/management/v3"
	"github.com/rancher/types/compose"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Formatter(apiContext *types.APIContext, resource *types.RawResource) {
	resource.AddAction(apiContext, "refresh")
	resource.Links["exportYaml"] = apiContext.URLBuilder.Link("exportYaml", resource)
}

func CollectionFormatter(request *types.APIContext, collection *types.GenericCollection) {
	collection.AddAction(request, "refresh")
}

type ActionHandler struct {
	CatalogClient        v3.CatalogInterface
	ProjectCatalogClient v3.ProjectCatalogInterface
	ClusterCatalogClient v3.ClusterCatalogInterface
}

func (a ActionHandler) RefreshActionHandler(actionName string, action *types.Action, apiContext *types.APIContext) error {
	if actionName != "refresh" {
		return httperror.NewAPIError(httperror.NotFound, "not found")
	}

	catalogs := []v3.Catalog{}
	if apiContext.ID != "" {
		catalog, err := a.CatalogClient.Get(apiContext.ID, metav1.GetOptions{})
		if err != nil {
			return err
		}
		catalogs = append(catalogs, *catalog)
	} else {
		catalogList, err := a.CatalogClient.List(metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, catalog := range catalogList.Items {
			catalogs = append(catalogs, catalog)
		}
	}
	for _, catalog := range catalogs {
		catalog.Status.LastRefreshTimestamp = time.Now().Format(time.RFC3339)
		v3.CatalogConditionRefreshed.Unknown(&catalog)
		if _, err := a.CatalogClient.Update(&catalog); err != nil {
			return err
		}
	}
	data := map[string]interface{}{
		"catalogs": catalogs,
	}
	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}

func (a ActionHandler) ExportYamlHandler(apiContext *types.APIContext, next types.RequestHandler) error {
	switch apiContext.Link {
	case "exportyaml":
		catalog, err := a.CatalogClient.Get(apiContext.ID, metav1.GetOptions{})
		if err != nil {
			return rpctypes.ErrGRPCStopped
		}
		topkey := compose.Config{}
		topkey.Version = "v3"
		ca := client.Catalog{}
		if err := convert.ToObj(catalog.Spec, &ca); err != nil {
			return err
		}
		topkey.Catalogs = map[string]client.Catalog{}
		topkey.Catalogs[catalog.Name] = ca
		m, err := convert.EncodeToMap(topkey)
		if err != nil {
			return err
		}
		delete(m["catalogs"].(map[string]interface{})[catalog.Name].(map[string]interface{}), "actions")
		delete(m["catalogs"].(map[string]interface{})[catalog.Name].(map[string]interface{}), "links")
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}

		buf, err := yaml.JSONToYAML(data)
		if err != nil {
			return err
		}
		reader := bytes.NewReader(buf)
		apiContext.Response.Header().Set("Content-Type", "text/yaml")
		http.ServeContent(apiContext.Response, apiContext.Request, "exportYaml", time.Now(), reader)
		return nil
	}
	return nil
}

func (a ActionHandler) RefreshProjectCatalogActionHandler(actionName string, action *types.Action, apiContext *types.APIContext) error {
	if actionName != "refresh" {
		return httperror.NewAPIError(httperror.NotFound, "not found")
	}

	prjCatalogs := []v3.ProjectCatalog{}
	if apiContext.ID != "" {
		ns, name := ref.Parse(apiContext.ID)
		catalog, err := a.ProjectCatalogClient.GetNamespaced(ns, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		prjCatalogs = append(prjCatalogs, *catalog)
	} else {
		catalogList, err := a.ProjectCatalogClient.List(metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, catalog := range catalogList.Items {
			prjCatalogs = append(prjCatalogs, catalog)
		}
	}
	for _, catalog := range prjCatalogs {
		catalog.Status.LastRefreshTimestamp = time.Now().Format(time.RFC3339)
		v3.CatalogConditionRefreshed.Unknown(&catalog)
		if _, err := a.ProjectCatalogClient.Update(&catalog); err != nil {
			return err
		}
	}
	data := map[string]interface{}{
		"catalogs": prjCatalogs,
	}
	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}

func (a ActionHandler) RefreshClusterCatalogActionHandler(actionName string, action *types.Action, apiContext *types.APIContext) error {
	if actionName != "refresh" {
		return httperror.NewAPIError(httperror.NotFound, "not found")
	}

	clCatalogs := []v3.ClusterCatalog{}
	if apiContext.ID != "" {
		ns, name := ref.Parse(apiContext.ID)
		catalog, err := a.ClusterCatalogClient.GetNamespaced(ns, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		clCatalogs = append(clCatalogs, *catalog)
	} else {
		catalogList, err := a.ClusterCatalogClient.List(metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, catalog := range catalogList.Items {
			clCatalogs = append(clCatalogs, catalog)
		}
	}
	for _, catalog := range clCatalogs {
		catalog.Status.LastRefreshTimestamp = time.Now().Format(time.RFC3339)
		v3.CatalogConditionRefreshed.Unknown(&catalog)
		if _, err := a.ClusterCatalogClient.Update(&catalog); err != nil {
			return err
		}
	}
	data := map[string]interface{}{
		"catalogs": clCatalogs,
	}
	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}
