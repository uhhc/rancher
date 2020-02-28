package clusterapi

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/steve/pkg/auth"
	"github.com/rancher/steve/pkg/client"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/schemaserver/server"
	"github.com/rancher/steve/pkg/schemaserver/types"
	steveserver "github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/server/store/proxy"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	projectv3 "github.com/rancher/types/apis/project.cattle.io/v3"
)

type Server struct {
	ctx          context.Context
	asl          accesscontrol.AccessSetLookup
	auth         auth.Middleware
	cf           *client.Factory
	clusterLinks []string
}

func (s *Server) Setup(ctx context.Context, server *steveserver.Server) error {
	s.ctx = ctx
	s.asl = server.AccessSetLookup
	s.auth = server.AuthMiddleware
	s.cf = server.ClientFactory

	server.SchemaTemplates = append(server.SchemaTemplates, schema.Template{
		ID: "management.cattle.io.v3.cluster",
		Formatter: func(request *types.APIRequest, resource *types.RawResource) {
			for _, link := range s.clusterLinks {
				resource.Links[link] = request.URLBuilder.Link(resource.Schema, resource.ID, link)
			}
		},
	})

	return nil
}

func (s *Server) newSchemas() *types.APISchemas {
	store := proxy.NewProxyStore(s.cf, s.asl)
	schemas := types.EmptyAPISchemas()

	schemas.MustImportAndCustomize(v3.Project{}, func(schema *types.APISchema) {
		schema.Store = store
		attributes.SetGroup(schema, v3.GroupName)
		attributes.SetVersion(schema, "v3")
		attributes.SetKind(schema, "Project")
		attributes.SetResource(schema, "projects")
		attributes.SetVerbs(schema, []string{"list", "get", "delete", "update", "watch", "patch"})
		s.clusterLinks = append(s.clusterLinks, "projects")
	})

	schemas.MustImportAndCustomize(projectv3.App{}, func(schema *types.APISchema) {
		schema.Store = &projectStore{
			Store:   store,
			clients: s.cf,
		}
		attributes.SetGroup(schema, projectv3.GroupName)
		attributes.SetVersion(schema, "v3")
		attributes.SetKind(schema, "App")
		attributes.SetResource(schema, "apps")
		attributes.SetVerbs(schema, []string{"list", "get", "delete", "update", "watch", "patch"})
		s.clusterLinks = append(s.clusterLinks, "apps")
	})

	return schemas
}

func (s *Server) newAPIHandler() http.Handler {
	server := server.DefaultAPIServer()
	for k, v := range server.ResponseWriters {
		server.ResponseWriters[k] = stripNS{writer: v}
	}

	s.clusterLinks = []string{
		"subscribe",
		"schemas",
	}

	sf := schema.NewCollection(s.ctx, server.Schemas, s.asl)
	sf.Reset(s.newSchemas().Schemas)

	return s.auth.Wrap(schema.WrapServer(sf, server))
}

func (s *Server) Wrap(next http.Handler) http.Handler {
	server := s.newAPIHandler()
	server = prefix(server)

	router := mux.NewRouter()
	router.UseEncodedPath()
	router.Path("/v1/management.cattle.io.v3.clusters/{namespace}/{type}").Handler(server)
	router.Path("/v1/management.cattle.io.v3.clusters/{namespace}/{type}/{name}").Handler(server)
	router.Path("/v1/management.cattle.io.v3.clusters/{namespace}/{type}/{namespace}/{name}").Handler(server)
	router.NotFoundHandler = next

	return router
}

func prefix(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)
		vars["prefix"] = "/v1/management.cattle.io.v3.clusters/" + vars["namespace"]
		next.ServeHTTP(rw, req)
	})
}
