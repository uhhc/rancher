package steve

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/uhhc/rancher/pkg/settings"
	"github.com/uhhc/rancher/pkg/steve/pkg/github"
	"github.com/uhhc/rancher/pkg/steve/pkg/proxy"
	"github.com/uhhc/rancher/pkg/wrangler"
	"github.com/rancher/steve/pkg/dashboard"
	steve "github.com/rancher/steve/pkg/server"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
)

func Setup(server *steve.Server, config *wrangler.Context, localSupport bool, rancherHandler http.Handler) error {
	githubHandler, err := github.NewProxy(config.Core.Secret().Cache(),
		settings.GithubProxyAPIURL.Get(),
		"cattle-system",
		"github")
	if err != nil {
		return err
	}

	cfg := authorizerfactory.DelegatingAuthorizerConfig{
		SubjectAccessReviewClient: config.K8s.AuthorizationV1().SubjectAccessReviews(),
		AllowCacheTTL:             time.Second * time.Duration(settings.AuthorizationCacheTTLSeconds.GetInt()),
		DenyCacheTTL:              time.Second * time.Duration(settings.AuthorizationDenyCacheTTLSeconds.GetInt()),
	}

	authorizer, err := cfg.New()
	if err != nil {
		return err
	}

	proxy := proxy.NewProxyHandler(authorizer,
		config.TunnelServer,
		config.Mgmt.Cluster().Cache())

	server.Next = newRouter(&handler{
		GitHub:       server.AuthMiddleware.Wrap(githubHandler),
		Proxy:        server.AuthMiddleware.Wrap(proxy),
		ProxyMatcher: proxy.MatchNonLegacy,
		Rancher:      rancherHandler,
		Steve:        server.Next,
	}, localSupport)

	// wrap with UI
	server.Next = dashboard.Route(server.Next, settings.DashboardIndex.Get)

	return nil
}

type handler struct {
	Rancher      http.Handler
	GitHub       http.Handler
	Proxy        http.Handler
	ProxyMatcher func(string, bool) mux.MatcherFunc
	Steve        http.Handler
}

func newRouter(h *handler, localSupport bool) http.Handler {
	mux := mux.NewRouter()
	mux.UseEncodedPath()
	mux.Handle("/v1/github{path:.*}", h.GitHub)
	mux.Path("/{prefix:k8s/clusters/[^/]+}{suffix:/v1.*}").MatcherFunc(h.ProxyMatcher("/k8s/clusters/", true)).Handler(h.Proxy)
	mux.Path("/{prefix:k8s/clusters/[^/]+}{suffix:.*}").MatcherFunc(h.ProxyMatcher("/k8s/clusters/", false)).Handler(h.Proxy)
	if localSupport {
		mux.NotFoundHandler = h.Steve
	} else {
		mux.PathPrefix("/v1/schemas").Handler(h.Steve)
		mux.PathPrefix("/v1/userpreference").Handler(h.Steve)
		mux.PathPrefix("/v1/management.cattle.io").Methods(http.MethodGet).Handler(h.Steve)
		mux.NotFoundHandler = h.Rancher
	}
	return mux
}
