package clusterregistrationtokens

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/norman/types"
	"github.com/rancher/norman/urlbuilder"
	"github.com/uhhc/rancher/pkg/image"
	"github.com/uhhc/rancher/pkg/settings"
	"github.com/uhhc/rancher/pkg/systemtemplate"
	"github.com/rancher/types/apis/management.cattle.io/v3/schema"
)

func ClusterImportHandler(resp http.ResponseWriter, req *http.Request) {
	resp.Header().Set("Content-Type", "text/plain")
	token := mux.Vars(req)["token"]

	urlBuilder, err := urlbuilder.New(req, schema.Version, types.NewSchemas())
	if err != nil {
		resp.WriteHeader(500)
		resp.Write([]byte(err.Error()))
		return
	}
	url := urlBuilder.RelativeToRoot("")

	authImage := ""
	authImages := req.URL.Query()["authImage"]
	if len(authImages) > 0 {
		authImage = authImages[0]
	}

	if err := systemtemplate.SystemTemplate(resp, image.Resolve(settings.AgentImage.Get()), authImage, "", token, url,
		false, nil, nil); err != nil {
		resp.WriteHeader(500)
		resp.Write([]byte(err.Error()))
	}
}
