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
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/apis/management.cattle.io/v3public"
	"github.com/rancher/types/client/management/v3"
	publicclient "github.com/rancher/types/client/management/v3public"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
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
	userMGR      common.UserManager
}

func Configure(ctx context.Context, mgmtCtx *config.ScaledContext, userMGR common.UserManager) common.AuthProvider {
	githubClient := &GClient{
		httpClient: &http.Client{},
	}

	return &ghProvider{
		ctx:          ctx,
		authConfigs:  mgmtCtx.Management.AuthConfigs(""),
		githubClient: githubClient,
		userMGR:      userMGR,
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

func (g *ghProvider) AuthenticateUser(input interface{}) (v3.Principal, []v3.Principal, map[string]string, error) {
	login, ok := input.(*v3public.GithubLogin)
	if !ok {
		return v3.Principal{}, nil, nil, errors.New("unexpected input type")
	}

	return g.LoginUser(login, nil)
}

func (g *ghProvider) LoginUser(githubCredential *v3public.GithubLogin, config *v3.GithubConfig) (v3.Principal, []v3.Principal, map[string]string, error) {
	var groupPrincipals []v3.Principal
	var userPrincipal v3.Principal
	var providerInfo = make(map[string]string)
	var err error

	if config == nil {
		config, err = g.getGithubConfigCR()
		if err != nil {
			return v3.Principal{}, nil, nil, err
		}
	}

	securityCode := githubCredential.Code

	logrus.Debugf("GitHubIdentityProvider AuthenticateUser called for securityCode %v", securityCode)
	accessToken, err := g.githubClient.getAccessToken(securityCode, config)
	if err != nil {
		logrus.Infof("Error generating accessToken from github %v", err)
		return v3.Principal{}, nil, nil, err
	}
	logrus.Debugf("Received AccessToken from github %v", accessToken)

	providerInfo["access_token"] = accessToken

	user, err := g.githubClient.getUser(accessToken, config)
	if err != nil {
		return v3.Principal{}, nil, nil, err
	}
	userPrincipal = v3.Principal{
		ObjectMeta:     metav1.ObjectMeta{Name: Name + "_user://" + strconv.Itoa(user.ID)},
		DisplayName:    user.Name,
		LoginName:      user.Login,
		PrincipalType:  "user",
		Provider:       Name,
		Me:             true,
		ProfilePicture: user.AvatarURL,
	}

	orgAccts, err := g.githubClient.getOrgs(accessToken, config)
	if err != nil {
		return v3.Principal{}, nil, nil, err
	}

	for _, orgAcct := range orgAccts {
		name := orgAcct.Name
		if name == "" {
			name = orgAcct.Login
		}
		groupPrincipal := v3.Principal{
			ObjectMeta:     metav1.ObjectMeta{Name: Name + "_org://" + strconv.Itoa(orgAcct.ID)},
			DisplayName:    name,
			PrincipalType:  "group",
			Provider:       Name,
			ProfilePicture: orgAcct.AvatarURL,
		}
		groupPrincipals = append(groupPrincipals, groupPrincipal)
	}

	teamAccts, err := g.githubClient.getTeams(accessToken, config)
	if err != nil {
		return v3.Principal{}, nil, nil, err
	}
	for _, teamAcct := range teamAccts {
		groupPrincipal := v3.Principal{
			ObjectMeta:     metav1.ObjectMeta{Name: Name + "_team://" + strconv.Itoa(teamAcct.ID)},
			DisplayName:    teamAcct.Name,
			PrincipalType:  "group",
			Provider:       Name,
			ProfilePicture: teamAcct.AvatarURL,
		}
		groupPrincipals = append(groupPrincipals, groupPrincipal)
	}

	allowed, err := g.userMGR.CheckAccess(config.AccessMode, config.AllowedPrincipalIDs, userPrincipal, groupPrincipals)
	if err != nil {
		return v3.Principal{}, nil, nil, err
	}
	if !allowed {
		return v3.Principal{}, nil, nil, httperror.NewAPIError(httperror.Unauthorized, "unauthorized")
	}

	return userPrincipal, groupPrincipals, providerInfo, nil
}

func (g *ghProvider) SearchPrincipals(searchKey, principalType string, myToken v3.Token) ([]v3.Principal, error) {
	var principals []v3.Principal
	var err error

	config, err := g.getGithubConfigCR()
	if err != nil {
		return principals, err
	}

	accessToken := myToken.ProviderInfo["access_token"]

	if principalType == "" || principalType == "user" {
		user, err := g.githubClient.getUserByName(searchKey, accessToken, config)
		if err == nil {
			userPrincipal := v3.Principal{
				ObjectMeta:     metav1.ObjectMeta{Name: Name + "_user://" + strconv.Itoa(user.ID)},
				DisplayName:    user.Name,
				LoginName:      user.Login,
				PrincipalType:  "user",
				Provider:       Name,
				Me:             false,
				ProfilePicture: user.AvatarURL,
			}
			if g.isThisUserMe(myToken.UserPrincipal, userPrincipal) {
				userPrincipal.Me = true
			}
			principals = append(principals, userPrincipal)
		}
	}

	if principalType == "" || principalType == "group" {
		orgAcct, err := g.githubClient.getOrgByName(searchKey, accessToken, config)
		if err == nil {
			groupPrincipal := v3.Principal{
				ObjectMeta:     metav1.ObjectMeta{Name: Name + "_org://" + strconv.Itoa(orgAcct.ID)},
				DisplayName:    orgAcct.Name,
				PrincipalType:  "group",
				Provider:       Name,
				ProfilePicture: orgAcct.AvatarURL,
			}
			if g.isMemberOf(myToken.GroupPrincipals, groupPrincipal) {
				groupPrincipal.MemberOf = true
			}
			principals = append(principals, groupPrincipal)
		}
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

	accessToken := token.ProviderInfo["access_token"]

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

	isUser := false

	switch principalType {
	case userType:
		isUser = true
		fallthrough
	case orgType:
		acct, err := g.githubClient.getUserOrgByID(externalID, accessToken, config)
		if err != nil {
			return v3.Principal{}, err
		}

		princ := v3.Principal{
			ObjectMeta:     metav1.ObjectMeta{Name: Name + "_" + principalType + "://" + strconv.Itoa(acct.ID)},
			DisplayName:    acct.Name,
			LoginName:      acct.Login,
			Provider:       Name,
			Me:             false,
			ProfilePicture: acct.AvatarURL,
		}
		if isUser {
			princ.Me = g.isThisUserMe(token.UserPrincipal, princ)
			princ.PrincipalType = "user"
		} else {
			princ.MemberOf = g.isMemberOf(token.GroupPrincipals, princ)
			princ.PrincipalType = "group"
		}
		return princ, nil
	case teamType:
		acct, err := g.githubClient.getTeamByID(externalID, accessToken, config)
		if err != nil {
			return v3.Principal{}, err
		}

		princ := v3.Principal{
			ObjectMeta:     metav1.ObjectMeta{Name: Name + "_" + teamType + "://" + strconv.Itoa(acct.ID)},
			DisplayName:    acct.Name,
			PrincipalType:  "group",
			Provider:       Name,
			ProfilePicture: acct.AvatarURL,
		}
		if g.isMemberOf(token.GroupPrincipals, princ) {
			princ.MemberOf = true
		}
		return princ, nil
	default:
		return v3.Principal{}, fmt.Errorf("Cannot get the github account due to invalid externalIDType %v", principalType)
	}
}

func (g *ghProvider) isThisUserMe(me v3.Principal, other v3.Principal) bool {

	if me.ObjectMeta.Name == other.ObjectMeta.Name && me.LoginName == other.LoginName && me.PrincipalType == other.PrincipalType {
		return true
	}
	return false
}

func (g *ghProvider) isMemberOf(myGroups []v3.Principal, other v3.Principal) bool {

	for _, mygroup := range myGroups {
		if mygroup.ObjectMeta.Name == other.ObjectMeta.Name && mygroup.PrincipalType == other.PrincipalType {
			return true
		}
	}
	return false
}
