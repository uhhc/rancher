package github

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/auth/providers/common"
	"github.com/rancher/rancher/pkg/auth/tokens"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/apis/management.cattle.io/v3public"
	"github.com/rancher/types/client/management/v3"
	publicclient "github.com/rancher/types/client/management/v3public"
	"github.com/rancher/types/config"
	"github.com/rancher/types/user"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	Name = "github"
)

type ghProvider struct {
	ctx          context.Context
	authConfigs  v3.AuthConfigInterface
	githubClient *GClient
	userMGR      user.Manager
	tokenMGR     *tokens.Manager
}

func Configure(ctx context.Context, mgmtCtx *config.ScaledContext, userMGR user.Manager, tokenMGR *tokens.Manager) common.AuthProvider {
	githubClient := &GClient{
		httpClient: &http.Client{},
	}

	return &ghProvider{
		ctx:          ctx,
		authConfigs:  mgmtCtx.Management.AuthConfigs(""),
		githubClient: githubClient,
		userMGR:      userMGR,
		tokenMGR:     tokenMGR,
	}
}

func (g *ghProvider) GetName() string {
	return Name
}

func (g *ghProvider) CustomizeSchema(schema *types.Schema) {
	schema.ActionHandler = g.actionHandler
	schema.Formatter = g.formatter
}

func (g *ghProvider) TransformToAuthProvider(authConfig map[string]interface{}) map[string]interface{} {
	p := common.TransformToAuthProvider(authConfig)
	p[publicclient.GithubProviderFieldRedirectURL] = formGithubRedirectURLFromMap(authConfig)
	return p
}

func (g *ghProvider) getGithubConfigCR() (*v3.GithubConfig, error) {
	authConfigObj, err := g.authConfigs.ObjectClient().UnstructuredClient().Get(Name, metav1.GetOptions{})
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

func (g *ghProvider) saveGithubConfig(config *v3.GithubConfig) error {
	storedGithubConfig, err := g.getGithubConfigCR()
	if err != nil {
		return err
	}
	config.APIVersion = "management.cattle.io/v3"
	config.Kind = v3.AuthConfigGroupVersionKind.Kind
	config.Type = client.GithubConfigType
	config.ObjectMeta = storedGithubConfig.ObjectMeta

	logrus.Debugf("updating githubConfig")
	_, err = g.authConfigs.ObjectClient().Update(config.ObjectMeta.Name, config)
	if err != nil {
		return err
	}
	return nil
}

func (g *ghProvider) AuthenticateUser(input interface{}) (v3.Principal, []v3.Principal, string, error) {
	login, ok := input.(*v3public.GithubLogin)
	if !ok {
		return v3.Principal{}, nil, "", errors.New("unexpected input type")
	}

	return g.LoginUser(login, nil, false)
}

func (g *ghProvider) LoginUser(githubCredential *v3public.GithubLogin, config *v3.GithubConfig, test bool) (v3.Principal, []v3.Principal, string, error) {
	var groupPrincipals []v3.Principal
	var userPrincipal v3.Principal
	var err error

	if config == nil {
		config, err = g.getGithubConfigCR()
		if err != nil {
			return v3.Principal{}, nil, "", err
		}
	}

	securityCode := githubCredential.Code

	logrus.Debugf("GitHubIdentityProvider AuthenticateUser called for securityCode %v", securityCode)
	accessToken, err := g.githubClient.getAccessToken(securityCode, config)
	if err != nil {
		logrus.Infof("Error generating accessToken from github %v", err)
		return v3.Principal{}, nil, "", err
	}
	logrus.Debugf("Received AccessToken from github %v", accessToken)

	user, err := g.githubClient.getUser(accessToken, config)
	if err != nil {
		return v3.Principal{}, nil, "", err
	}
	userPrincipal = g.toPrincipal(userType, user, nil)
	userPrincipal.Me = true

	orgAccts, err := g.githubClient.getOrgs(accessToken, config)
	if err != nil {
		return v3.Principal{}, nil, "", err
	}
	for _, orgAcct := range orgAccts {
		groupPrincipal := g.toPrincipal(orgType, orgAcct, nil)
		groupPrincipal.MemberOf = true
		groupPrincipals = append(groupPrincipals, groupPrincipal)
	}

	teamAccts, err := g.githubClient.getTeams(accessToken, config)
	if err != nil {
		return v3.Principal{}, nil, "", err
	}
	for _, teamAcct := range teamAccts {
		groupPrincipal := g.toPrincipal(teamType, teamAcct, nil)
		groupPrincipal.MemberOf = true
		groupPrincipals = append(groupPrincipals, groupPrincipal)
	}

	testAllowedPrincipals := config.AllowedPrincipalIDs
	if test && config.AccessMode == "restricted" {
		testAllowedPrincipals = append(testAllowedPrincipals, userPrincipal.Name)
	}

	allowed, err := g.userMGR.CheckAccess(config.AccessMode, testAllowedPrincipals, userPrincipal, groupPrincipals)
	if err != nil {
		return v3.Principal{}, nil, "", err
	}
	if !allowed {
		return v3.Principal{}, nil, "", httperror.NewAPIError(httperror.Unauthorized, "unauthorized")
	}

	return userPrincipal, groupPrincipals, accessToken, nil
}

func (g *ghProvider) SearchPrincipals(searchKey, principalType string, token v3.Token) ([]v3.Principal, error) {
	var principals []v3.Principal
	var err error

	config, err := g.getGithubConfigCR()
	if err != nil {
		return principals, err
	}

	accessToken, err := g.tokenMGR.GetSecret(&token)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		accessToken = token.ProviderInfo["access_token"]
	}

	accts, err := g.githubClient.searchUsers(searchKey, principalType, accessToken, config)
	if err != nil {
		logrus.Errorf("problem searching github: %v", err)
	}

	for _, acct := range accts {
		pType := strings.ToLower(acct.Type)
		if pType == "organization" {
			pType = orgType
		}
		p := g.toPrincipal(pType, acct, &token)
		principals = append(principals, p)
	}

	return principals, nil
}

const (
	userType = "user"
	teamType = "team"
	orgType  = "org"
)

func (g *ghProvider) GetPrincipal(principalID string, token v3.Token) (v3.Principal, error) {
	config, err := g.getGithubConfigCR()
	if err != nil {
		return v3.Principal{}, err
	}

	accessToken, err := g.tokenMGR.GetSecret(&token)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return v3.Principal{}, err
		}
		accessToken = token.ProviderInfo["access_token"]
	}
	// parsing id to get the external id and type. id looks like github_[user|org|team]://12345
	var externalID string
	parts := strings.SplitN(principalID, ":", 2)
	if len(parts) != 2 {
		return v3.Principal{}, errors.Errorf("invalid id %v", principalID)
	}
	externalID = strings.TrimPrefix(parts[1], "//")
	parts = strings.SplitN(parts[0], "_", 2)
	if len(parts) != 2 {
		return v3.Principal{}, errors.Errorf("invalid id %v", principalID)
	}

	principalType := parts[1]
	var acct Account
	switch principalType {
	case userType:
		fallthrough
	case orgType:
		acct, err = g.githubClient.getUserOrgByID(externalID, accessToken, config)
		if err != nil {
			return v3.Principal{}, err
		}
	case teamType:
		acct, err = g.githubClient.getTeamByID(externalID, accessToken, config)
		if err != nil {
			return v3.Principal{}, err
		}
	default:
		return v3.Principal{}, fmt.Errorf("Cannot get the github account due to invalid externalIDType %v", principalType)
	}

	princ := g.toPrincipal(principalType, acct, &token)
	return princ, nil
}

func (g *ghProvider) toPrincipal(principalType string, acct Account, token *v3.Token) v3.Principal {
	displayName := acct.Name
	if displayName == "" {
		displayName = acct.Login
	}

	princ := v3.Principal{
		ObjectMeta:     metav1.ObjectMeta{Name: Name + "_" + principalType + "://" + strconv.Itoa(acct.ID)},
		DisplayName:    displayName,
		LoginName:      acct.Login,
		Provider:       Name,
		Me:             false,
		ProfilePicture: acct.AvatarURL,
	}

	if principalType == userType {
		princ.PrincipalType = "user"
		if token != nil {
			princ.Me = g.isThisUserMe(token.UserPrincipal, princ)
		}
	} else {
		princ.PrincipalType = "group"
		if token != nil {
			princ.MemberOf = g.tokenMGR.IsMemberOf(*token, princ)
		}
	}

	return princ
}

func (g *ghProvider) isThisUserMe(me v3.Principal, other v3.Principal) bool {

	if me.ObjectMeta.Name == other.ObjectMeta.Name && me.LoginName == other.LoginName && me.PrincipalType == other.PrincipalType {
		return true
	}
	return false
}
