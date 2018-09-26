package saml

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/gorilla/mux"
	"github.com/rancher/rancher/pkg/auth/tokens"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IDPMetadata struct {
	XMLName           xml.Name                `xml:"urn:oasis:names:tc:SAML:2.0:metadata EntityDescriptor"`
	ValidUntil        time.Time               `xml:"validUntil,attr"`
	EntityID          string                  `xml:"entityID,attr"`
	IDPSSODescriptors []saml.IDPSSODescriptor `xml:"IDPSSODescriptor"`
	SPSSODescriptors  []saml.SPSSODescriptor  `xml:"SPSSODescriptor"`
}

var root *mux.Router
var appliedVersion string
var initMu sync.Mutex

func InitializeSamlServiceProvider(configToSet *v3.SamlConfig, name string) error {

	initMu.Lock()
	defer initMu.Unlock()

	if configToSet.ResourceVersion == appliedVersion {
		return nil
	}

	var privKey *rsa.PrivateKey
	var cert *x509.Certificate
	var err error
	var ok bool

	if configToSet.IDPMetadataContent == "" {
		return fmt.Errorf("SAML: Cannot initialize saml SP properly, missing IDP URL/metadata in the config %v", configToSet)
	}

	if configToSet.SpCert == "" {
		return fmt.Errorf("SAML: Cannot initialize saml SP properly, missing SpCert in the config %v", configToSet)
	}

	if configToSet.SpKey == "" {
		return fmt.Errorf("SAML: Cannot initialize saml SP properly, missing SpKey in the config %v", configToSet)
	}

	if configToSet.SpKey != "" {
		// used from ssh.ParseRawPrivateKey

		block, _ := pem.Decode([]byte(configToSet.SpKey))
		if block == nil {
			return fmt.Errorf("SAML: no key found")
		}

		if strings.Contains(block.Headers["Proc-Type"], "ENCRYPTED") {
			return fmt.Errorf("SAML: cannot decode encrypted private keys")
		}

		switch block.Type {
		case "RSA PRIVATE KEY":
			privKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return fmt.Errorf("SAML: error parsing PKCS1 RSA key: %v", err)
			}
		case "PRIVATE KEY":
			pk, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return fmt.Errorf("SAML: error parsing PKCS8 RSA key: %v", err)
			}
			privKey, ok = pk.(*rsa.PrivateKey)
			if !ok {
				return fmt.Errorf("SAML: unable to get rsa key")
			}
		default:
			return fmt.Errorf("SAML: unsupported key type %q", block.Type)
		}
	}

	if configToSet.SpCert != "" {
		block, _ := pem.Decode([]byte(configToSet.SpCert))
		if block == nil {
			return fmt.Errorf("SAML: failed to parse PEM block containing the private key")
		}

		cert, err = x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("SAML: failed to parse DER encoded public key: " + err.Error())
		}
	}

	provider := SamlProviders[name]

	rancherAPIHost := strings.TrimRight(configToSet.RancherAPIHost, "/")
	samlURL := rancherAPIHost + "/v1-saml/"
	samlURL += name
	actURL, err := url.Parse(samlURL)
	if err != nil {
		return fmt.Errorf("SAML: error in parsing URL")
	}

	metadataURL := *actURL
	metadataURL.Path = metadataURL.Path + "/saml/metadata"
	acsURL := *actURL
	acsURL.Path = acsURL.Path + "/saml/acs"

	sp := saml.ServiceProvider{
		Key:         privKey,
		Certificate: cert,
		MetadataURL: metadataURL,
		AcsURL:      acsURL,
	}

	// XML unmarshal throws an error for IdP Metadata cacheDuration field, as it's of type xml Duration. Using a separate struct for unmarshaling for now
	idm := &IDPMetadata{}
	if configToSet.IDPMetadataContent != "" {
		sp.IDPMetadata = &saml.EntityDescriptor{}
		if err := xml.NewDecoder(strings.NewReader(configToSet.IDPMetadataContent)).Decode(idm); err != nil {
			return fmt.Errorf("SAML: cannot initialize saml SP, cannot decode IDP Metadata content from the config %v, error %v", configToSet, err)
		}
	}

	sp.IDPMetadata.XMLName = idm.XMLName
	sp.IDPMetadata.ValidUntil = idm.ValidUntil
	sp.IDPMetadata.EntityID = idm.EntityID
	sp.IDPMetadata.SPSSODescriptors = idm.SPSSODescriptors
	sp.IDPMetadata.IDPSSODescriptors = idm.IDPSSODescriptors
	if name == ADFSName {
		sp.AuthnNameIDFormat = saml.UnspecifiedNameIDFormat
	}

	provider.serviceProvider = &sp

	cookieStore := samlsp.ClientCookies{
		ServiceProvider: &sp,
		Name:            "token",
		Domain:          actURL.Host,
	}
	provider.clientState = &cookieStore

	SamlProviders[name] = provider

	switch name {
	case PingName:
		root.Get("PingACS").HandlerFunc(provider.ServeHTTP)
		root.Get("PingMetadata").HandlerFunc(provider.ServeHTTP)
	case ADFSName:
		root.Get("AdfsACS").HandlerFunc(provider.ServeHTTP)
		root.Get("AdfsMetadata").HandlerFunc(provider.ServeHTTP)
	case KeyCloakName:
		root.Get("KeyCloakACS").HandlerFunc(provider.ServeHTTP)
		root.Get("KeyCloakMetadata").HandlerFunc(provider.ServeHTTP)
	}

	appliedVersion = configToSet.ResourceVersion

	return nil
}

func AuthHandler() http.Handler {
	root = mux.NewRouter()
	root.Methods("POST").Path("/v1-saml/ping/saml/acs").Name("PingACS")
	root.Methods("GET").Path("/v1-saml/ping/saml/metadata").Name("PingMetadata")

	root.Methods("POST").Path("/v1-saml/adfs/saml/acs").Name("AdfsACS")
	root.Methods("GET").Path("/v1-saml/adfs/saml/metadata").Name("AdfsMetadata")

	root.Methods("POST").Path("/v1-saml/keycloak/saml/acs").Name("KeyCloakACS")
	root.Methods("GET").Path("/v1-saml/keycloak/saml/metadata").Name("KeyCloakMetadata")

	return root
}

func (s *Provider) getSamlPrincipals(config *v3.SamlConfig, samlData map[string][]string) (v3.Principal, []v3.Principal) {
	var userPrincipal v3.Principal
	var groupPrincipals []v3.Principal
	uid, ok := samlData[config.UIDField]
	if ok {
		userPrincipal = v3.Principal{
			ObjectMeta:    metav1.ObjectMeta{Name: s.userType + "://" + uid[0]},
			Provider:      s.name,
			PrincipalType: "user",
			Me:            true,
		}

		displayName, ok := samlData[config.DisplayNameField]
		if ok {
			userPrincipal.DisplayName = displayName[0]
		}

		userName, ok := samlData[config.UserNameField]
		if ok {
			userPrincipal.LoginName = userName[0]
		}

		groups, ok := samlData[config.GroupsField]
		if ok {
			for _, group := range groups {
				group := v3.Principal{
					ObjectMeta:    metav1.ObjectMeta{Name: s.groupType + "://" + group},
					DisplayName:   group,
					Provider:      s.name,
					PrincipalType: "group",
					MemberOf:      true,
				}
				groupPrincipals = append(groupPrincipals, group)
			}
		}
	}
	return userPrincipal, groupPrincipals
}

// HandleSamlAssertion processes/handles the assertion obtained by the POST to /saml/acs from IdP
func (s *Provider) HandleSamlAssertion(w http.ResponseWriter, r *http.Request, assertion *saml.Assertion) {
	var groupPrincipals []v3.Principal
	var userPrincipal v3.Principal

	if relayState := r.Form.Get("RelayState"); relayState != "" {
		// delete the cookie
		s.clientState.DeleteState(w, r, relayState)
	}

	redirectURL := s.clientState.GetState(r, "Rancher_FinalRedirectURL")

	samlData := make(map[string][]string)

	for _, attributeStatement := range assertion.AttributeStatements {
		for _, attr := range attributeStatement.Attributes {
			attrName := attr.FriendlyName
			if attrName == "" {
				attrName = attr.Name
			}
			for _, value := range attr.Values {
				samlData[attrName] = append(samlData[attrName], value.Value)
			}
		}
	}

	config, err := s.getSamlConfig()
	if err != nil {
		log.Errorf("SAML: Error getting saml config %v", err)
		http.Redirect(w, r, redirectURL+"/login?errorCode=500", http.StatusFound)
		return
	}

	userPrincipal, groupPrincipals = s.getSamlPrincipals(config, samlData)
	allowedPrincipals := config.AllowedPrincipalIDs

	allowed, err := s.userMGR.CheckAccess(config.AccessMode, allowedPrincipals, userPrincipal, groupPrincipals)
	if err != nil {
		log.Errorf("SAML: Error during login while checking access %v", err)
		http.Redirect(w, r, redirectURL+"/login?errorCode=500", http.StatusFound)
		return
	}
	if !allowed {
		log.Errorf("SAML: User does not have access %v", err)
		http.Redirect(w, r, redirectURL+"/login?errorCode=403", http.StatusFound)
		return
	}

	userID := s.clientState.GetState(r, "Rancher_UserID")
	rancherAction := s.clientState.GetState(r, "Rancher_Action")
	if userID != "" && rancherAction == "testAndEnable" {
		user, err := s.userMGR.SetPrincipalOnCurrentUserByUserID(userID, userPrincipal)
		if err != nil {
			log.Errorf("SAML: Error setting principal on current user %v", err)
			http.Redirect(w, r, redirectURL+"/login?errorCode=500", http.StatusFound)
			return
		}

		config.Enabled = true
		err = s.saveSamlConfig(config)
		if err != nil {
			log.Errorf("SAML: Error saving saml config %v", err)
			http.Redirect(w, r, redirectURL+"/login?errorCode=500", http.StatusFound)
			return
		}

		isSecure := false
		if r.URL.Scheme == "https" {
			isSecure = true
		}
		err = setRancherToken(w, r, s.tokenMGR, user.Name, userPrincipal, groupPrincipals, isSecure)
		if err != nil {
			log.Errorf("SAML: Failed creating token with error: %v", err)
			http.Redirect(w, r, redirectURL+"/login?errorCode=500", http.StatusFound)
		}
		// delete the cookies
		s.clientState.DeleteState(w, r, "Rancher_UserID")
		s.clientState.DeleteState(w, r, "Rancher_Action")
		redirectURL := s.clientState.GetState(r, "Rancher_FinalRedirectURL")
		s.clientState.DeleteState(w, r, "Rancher_FinalRedirectURL")
		if redirectURL != "" {
			// delete the cookie
			s.clientState.DeleteState(w, r, "Rancher_FinalRedirectURL")
			http.Redirect(w, r, redirectURL, http.StatusFound)
		}
		return
	}

	displayName := userPrincipal.DisplayName
	if displayName == "" {
		displayName = userPrincipal.LoginName
	}
	user, err := s.userMGR.EnsureUser(userPrincipal.Name, displayName)
	if err != nil {
		log.Errorf("SAML: Failed getting user with error: %v", err)
		http.Redirect(w, r, redirectURL+"/login?errorCode=500", http.StatusFound)
		return
	}

	if user.Enabled != nil && !*user.Enabled {
		log.Errorf("SAML: User %v permission denied", user.Name)
		http.Redirect(w, r, redirectURL+"/login?errorCode=403", http.StatusFound)
		return
	}

	err = setRancherToken(w, r, s.tokenMGR, user.Name, userPrincipal, groupPrincipals, true)
	if err != nil {
		log.Errorf("SAML: Failed creating token with error: %v", err)
		http.Redirect(w, r, redirectURL+"/login?errorCode=500", http.StatusFound)
	}
	redirectURL = s.clientState.GetState(r, "Rancher_FinalRedirectURL")
	if redirectURL != "" {
		// delete the cookie
		s.clientState.DeleteState(w, r, "Rancher_FinalRedirectURL")
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
	return
}

func setRancherToken(w http.ResponseWriter, r *http.Request, tokenMGR *tokens.Manager, userID string, userPrincipal v3.Principal,
	groupPrincipals []v3.Principal, isSecure bool) error {
	rToken, err := tokenMGR.NewLoginToken(userID, userPrincipal, groupPrincipals, "", 0, "")
	if err != nil {
		return err
	}
	tokenCookie := &http.Cookie{
		Name:     "R_SESS",
		Value:    rToken.ObjectMeta.Name + ":" + rToken.Token,
		Secure:   isSecure,
		Path:     "/",
		HttpOnly: true,
	}
	http.SetCookie(w, tokenCookie)
	return nil
}
