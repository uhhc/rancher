package ldap

import (
	"crypto/x509"
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types/slice"
	"github.com/rancher/rancher/pkg/auth/providers/common/ldap"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/apis/management.cattle.io/v3public"
	"github.com/sirupsen/logrus"
	ldapv2 "gopkg.in/ldap.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var operationalAttrList = []string{"1.1", "+", "*"}

func (p *ldapProvider) loginUser(credential *v3public.BasicLogin, config *v3.LdapConfig, caPool *x509.CertPool) (v3.Principal, []v3.Principal, error) {
	logrus.Debug("Now generating Ldap token")

	username := credential.Username
	password := credential.Password

	if password == "" {
		return v3.Principal{}, nil, httperror.NewAPIError(httperror.MissingRequired, "password not provided")
	}

	lConn, err := p.ldapConnection(config, caPool)
	if err != nil {
		return v3.Principal{}, nil, err
	}
	defer lConn.Close()

	serviceAccountPassword := config.ServiceAccountPassword
	serviceAccountUserName := config.ServiceAccountDistinguishedName
	err = ldap.AuthenticateServiceAccountUser(serviceAccountPassword, serviceAccountUserName, "", lConn)
	if err != nil {
		return v3.Principal{}, nil, err
	}

	logrus.Debug("Binding username password")

	searchRequest := ldapv2.NewSearchRequest(config.UserSearchBase,
		ldapv2.ScopeWholeSubtree, ldapv2.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(%v=%v)(%v=%v))", ObjectClass, config.UserObjectClass, config.UserLoginAttribute, ldapv2.EscapeFilter(username)),
		ldap.GetUserSearchAttributesForLDAP(ObjectClass, config), nil)
	result, err := lConn.Search(searchRequest)
	if err != nil {
		return v3.Principal{}, nil, httperror.WrapAPIError(err, httperror.Unauthorized, "authentication failed") // need to reload this error
	}

	if len(result.Entries) < 1 {
		return v3.Principal{}, nil, httperror.WrapAPIError(err, httperror.Unauthorized, "Cannot locate user information for "+searchRequest.Filter)
	} else if len(result.Entries) > 1 {
		return v3.Principal{}, nil, fmt.Errorf("ldap user search found more than one result")
	}

	userDN := result.Entries[0].DN //userDN is externalID
	err = lConn.Bind(userDN, password)
	if err != nil {
		if ldapv2.IsErrorWithCode(err, ldapv2.LDAPResultInvalidCredentials) {
			return v3.Principal{}, nil, httperror.WrapAPIError(err, httperror.Unauthorized, "authentication failed: invalid credentials")
		}
		return v3.Principal{}, nil, httperror.WrapAPIError(err, httperror.ServerError, "server error while authenticating")
	}

	searchOpRequest := ldapv2.NewSearchRequest(userDN,
		ldapv2.ScopeBaseObject, ldapv2.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(%v=%v)", ObjectClass, config.UserObjectClass),
		operationalAttrList, nil)
	opResult, err := lConn.Search(searchOpRequest)
	if err != nil {
		return v3.Principal{}, nil, httperror.WrapAPIError(err, httperror.Unauthorized, "authentication failed") // need to reload this error
	}

	if len(opResult.Entries) < 1 {
		return v3.Principal{}, nil, httperror.WrapAPIError(err, httperror.Unauthorized, "Cannot locate user information for "+searchOpRequest.Filter)
	}

	userPrincipal, groupPrincipals, err := p.getPrincipalsFromSearchResult(result, opResult, config, lConn)
	if err != nil {
		return v3.Principal{}, nil, err
	}

	allowed, err := p.userMGR.CheckAccess(config.AccessMode, config.AllowedPrincipalIDs, userPrincipal, groupPrincipals)
	if err != nil {
		return v3.Principal{}, nil, err
	}
	if !allowed {
		return v3.Principal{}, nil, httperror.NewAPIError(httperror.PermissionDenied, "Permission denied")
	}

	return userPrincipal, groupPrincipals, err
}

func (p *ldapProvider) getPrincipalsFromSearchResult(result *ldapv2.SearchResult, opResult *ldapv2.SearchResult, config *v3.LdapConfig, lConn *ldapv2.Conn) (v3.Principal, []v3.Principal, error) {
	var groupPrincipals []v3.Principal
	var userPrincipal v3.Principal
	var nonDupGroupPrincipals []v3.Principal
	var userScope, groupScope string
	var nestedGroupPrincipals []v3.Principal

	groupMap := make(map[string]bool)
	entry := result.Entries[0]
	userAttributes := entry.Attributes

	if !p.permissionCheck(userAttributes, config) {
		return v3.Principal{}, nil, fmt.Errorf("Permission denied")
	}

	logrus.Debugf("getPrincipals: user attributes: %v ", userAttributes)

	userMemberAttribute := entry.GetAttributeValues(config.UserMemberAttribute)
	if len(userMemberAttribute) == 0 {
		userMemberAttribute = opResult.Entries[0].GetAttributeValues(config.UserMemberAttribute)
	}

	logrus.Debugf("SearchResult memberOf attribute {%s}", userMemberAttribute)

	isType := false
	objectClass := entry.GetAttributeValues(ObjectClass)
	for _, obj := range objectClass {
		if strings.EqualFold(string(obj), config.UserObjectClass) {
			isType = true
		}
	}
	if !isType {
		return v3.Principal{}, nil, nil
	}

	userScope = p.userScope
	groupScope = p.groupScope

	user, err := ldap.AttributesToPrincipal(entry.Attributes, result.Entries[0].DN, userScope, p.providerName, config.UserObjectClass, config.UserNameAttribute, config.UserLoginAttribute, config.GroupObjectClass, config.GroupNameAttribute)
	if err != nil {
		return v3.Principal{}, groupPrincipals, err
	}

	userPrincipal = *user

	if len(userMemberAttribute) > 0 {
		for i := 0; i < len(userMemberAttribute); i += 50 {
			batchGroupDN := userMemberAttribute[i:ldap.Min(i+50, len(userMemberAttribute))]
			filter := fmt.Sprintf("(%v=%v)", ObjectClass, config.GroupObjectClass)
			query := "(|"
			for _, gdn := range batchGroupDN {
				query += fmt.Sprintf("(%v=%v)", config.GroupDNAttribute, ldapv2.EscapeFilter(gdn))
			}
			query += ")"
			query = fmt.Sprintf("(&%v%v)", filter, query)
			// Pulling user's groups
			logrus.Debugf("Ldap: Query for pulling user's groups: %v", query)
			userMemberGroupPrincipals, err := p.searchLdap(query, groupScope, config, lConn)
			groupPrincipals = append(groupPrincipals, userMemberGroupPrincipals...)
			if err != nil {
				return userPrincipal, groupPrincipals, err
			}
		}
	}

	opEntry := opResult.Entries[0]
	opAttributes := opEntry.Attributes

	groupMemberUserAttribute := entry.GetAttributeValues(config.GroupMemberUserAttribute)
	if len(groupMemberUserAttribute) == 0 {
		for _, attr := range opAttributes {
			if attr.Name == config.GroupMemberUserAttribute {
				groupMemberUserAttribute = attr.Values
			}
		}
	}

	if len(groupMemberUserAttribute) > 0 {
		query := fmt.Sprintf("(&(%v=%v)(%v=%v))", config.GroupMemberMappingAttribute, ldapv2.EscapeFilter(groupMemberUserAttribute[0]), ObjectClass, config.GroupObjectClass)
		newGroupPrincipals, err := p.searchLdap(query, groupScope, config, lConn)
		//deduplicate groupprincipals get from userMemberAttribute
		nonDupGroupPrincipals = p.findNonDuplicateBetweenGroupPrincipals(newGroupPrincipals, groupPrincipals, nonDupGroupPrincipals)
		groupPrincipals = append(groupPrincipals, nonDupGroupPrincipals...)
		if err != nil {
			return userPrincipal, groupPrincipals, err
		}
	}
	// Handle nestedgroups for openldap, filter operationalAttrList already handles nestedgroups for freeipa
	if config.NestedGroupMembershipEnabled && groupScope == "openldap_group" {
		searchDomain := config.UserSearchBase
		if config.GroupSearchBase != "" {
			searchDomain = config.GroupSearchBase
		}

		// Handling nestedgroups: tracing from down to top in order to find the parent groups, parent parent groups, and so on...
		// When traversing up, we note down all the parent groups and add them to groupPrincipals
		for _, groupPrincipal := range groupPrincipals {
			err = p.gatherParentGroups(groupPrincipal, searchDomain, groupScope, config, lConn, groupMap, &nestedGroupPrincipals)
			if err != nil {
				return userPrincipal, groupPrincipals, nil
			}
		}
		nonDupGroupPrincipals = p.findNonDuplicateBetweenGroupPrincipals(nestedGroupPrincipals, groupPrincipals, []v3.Principal{})
		groupPrincipals = append(groupPrincipals, nonDupGroupPrincipals...)
	}

	return userPrincipal, groupPrincipals, nil
}

func (p *ldapProvider) gatherParentGroups(groupPrincipal v3.Principal, searchDomain string, groupScope string, config *v3.LdapConfig, lConn *ldapv2.Conn, groupMap map[string]bool, nestedGroupPrincipals *[]v3.Principal) error {
	groupMap[groupPrincipal.ObjectMeta.Name] = true
	principals := []v3.Principal{}
	parts := strings.SplitN(groupPrincipal.ObjectMeta.Name, ":", 2)
	if len(parts) != 2 {
		return errors.Errorf("invalid id %v", groupPrincipal.ObjectMeta.Name)
	}
	groupDN := strings.TrimPrefix(parts[1], "//")

	searchGroup := ldapv2.NewSearchRequest(searchDomain,
		ldapv2.ScopeWholeSubtree, ldapv2.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(%v=%v)(%v=%v))", config.GroupMemberMappingAttribute, ldapv2.EscapeFilter(groupDN), ObjectClass, config.GroupObjectClass),
		ldap.GetGroupSearchAttributesForLDAP(ObjectClass, config), nil)
	resultGroups, err := lConn.Search(searchGroup)
	if err != nil {
		return err
	}

	for i := 0; i < len(resultGroups.Entries); i++ {
		entry := resultGroups.Entries[i]
		principal, err := ldap.AttributesToPrincipal(entry.Attributes, entry.DN, groupScope, p.providerName, config.UserObjectClass, config.UserNameAttribute, config.UserLoginAttribute, config.GroupObjectClass, config.GroupNameAttribute)
		if err != nil {
			logrus.Errorf("Error translating group result: %v", err)
			continue
		}
		principals = append(principals, *principal)
	}

	for _, gp := range principals {
		if _, ok := groupMap[gp.ObjectMeta.Name]; ok {
			continue
		} else {
			*nestedGroupPrincipals = append(*nestedGroupPrincipals, gp)
			err = p.gatherParentGroups(gp, searchDomain, groupScope, config, lConn, groupMap, nestedGroupPrincipals)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *ldapProvider) findNonDuplicateBetweenGroupPrincipals(newGroupPrincipals []v3.Principal, groupPrincipals []v3.Principal, nonDupGroupPrincipals []v3.Principal) []v3.Principal {
	for _, gp := range newGroupPrincipals {
		counter := 0
		for _, usermembergp := range groupPrincipals {
			// check the groups ObjectMeta.Name and name fields value are the same, then they are the same group
			if gp.ObjectMeta.Name == usermembergp.ObjectMeta.Name && gp.DisplayName == usermembergp.DisplayName {
				break
			} else {
				counter++
			}
		}
		if counter == len(groupPrincipals) {
			nonDupGroupPrincipals = append(nonDupGroupPrincipals, gp)
		}
	}
	return nonDupGroupPrincipals
}

func (p *ldapProvider) getPrincipal(distinguishedName string, scope string, config *v3.LdapConfig, caPool *x509.CertPool) (*v3.Principal, error) {
	var search *ldapv2.SearchRequest
	var filter string
	if !slice.ContainsString(freeIpaScopes, scope) && !slice.ContainsString(openLdapScopes, scope) {
		return nil, fmt.Errorf("Invalid scope")
	}

	var attributes []*ldapv2.AttributeTypeAndValue
	var attribs []*ldapv2.EntryAttribute
	object, err := ldapv2.ParseDN(distinguishedName)
	if err != nil {
		return nil, err
	}

	for _, rdns := range object.RDNs {
		for _, attr := range rdns.Attributes {
			attributes = append(attributes, attr)
			entryAttr := ldapv2.NewEntryAttribute(attr.Type, []string{attr.Value})
			attribs = append(attribs, entryAttr)
		}
	}

	if !ldap.IsType(attribs, scope) && !p.permissionCheck(attribs, config) {
		logrus.Errorf("Failed to get object %v", distinguishedName)
		return nil, nil
	}

	entityType := strings.Split(scope, "_")[1]
	if strings.EqualFold("user", entityType) {
		filter = fmt.Sprintf("(%v=%v)", ObjectClass, config.UserObjectClass)
	} else {
		filter = fmt.Sprintf("(%v=%v)", ObjectClass, config.GroupObjectClass)
	}

	logrus.Debugf("Query for getPrincipal(%v): %v", distinguishedName, filter)

	lConn, err := p.ldapConnection(config, caPool)
	if err != nil {
		return nil, err
	}
	defer lConn.Close()
	// Bind before query
	// If service acc bind fails, and auth is on, return principal formed using DN
	serviceAccountUsername := ldap.GetUserExternalID(config.ServiceAccountDistinguishedName, "")
	err = lConn.Bind(serviceAccountUsername, config.ServiceAccountPassword)

	if err != nil {
		if ldapv2.IsErrorWithCode(err, ldapv2.LDAPResultInvalidCredentials) && config.Enabled {
			var kind string
			if strings.EqualFold("user", entityType) {
				kind = "user"
			} else if strings.EqualFold("group", entityType) {
				kind = "group"
			}
			principal := &v3.Principal{
				ObjectMeta:    metav1.ObjectMeta{Name: scope + "://" + distinguishedName},
				DisplayName:   distinguishedName,
				LoginName:     distinguishedName,
				PrincipalType: kind,
			}

			return principal, nil
		}
		return nil, fmt.Errorf("Error in ldap bind: %v", err)
	}

	if strings.EqualFold("user", entityType) {
		search = ldapv2.NewSearchRequest(distinguishedName,
			ldapv2.ScopeBaseObject, ldapv2.NeverDerefAliases, 0, 0, false,
			filter,
			ldap.GetUserSearchAttributesForLDAP(ObjectClass, config), nil)
	} else {
		search = ldapv2.NewSearchRequest(distinguishedName,
			ldapv2.ScopeBaseObject, ldapv2.NeverDerefAliases, 0, 0, false,
			filter,
			ldap.GetGroupSearchAttributesForLDAP(ObjectClass, config), nil)
	}

	result, err := lConn.Search(search)
	if err != nil {
		if ldapErr, ok := err.(*ldapv2.Error); ok && ldapErr.ResultCode == 32 {
			return nil, httperror.NewAPIError(httperror.NotFound, fmt.Sprintf("%v not found", distinguishedName))
		}
		return nil, httperror.WrapAPIError(errors.Wrapf(err, "server returned error for search %v %v: %v", search.BaseDN, filter, err), httperror.ServerError, "Internal server error")
	}

	if len(result.Entries) < 1 {
		return nil, fmt.Errorf("No identities can be retrieved")
	} else if len(result.Entries) > 1 {
		return nil, fmt.Errorf("More than one result found")
	}

	entry := result.Entries[0]
	entryAttributes := entry.Attributes

	if !p.permissionCheck(entry.Attributes, config) {
		return nil, fmt.Errorf("Permission denied")
	}

	principal, err := ldap.AttributesToPrincipal(entryAttributes, distinguishedName, scope, p.providerName, config.UserObjectClass, config.UserNameAttribute, config.UserLoginAttribute, config.GroupObjectClass, config.GroupNameAttribute)
	if err != nil {
		return nil, err
	}
	return principal, nil
}

func (p *ldapProvider) searchPrincipals(name, principalType string, config *v3.LdapConfig, lConn *ldapv2.Conn) ([]v3.Principal, error) {
	name = ldapv2.EscapeFilter(name)

	var principals []v3.Principal

	if principalType == "" || principalType == "user" {
		userPrincipals, err := p.searchUser(name, config, lConn)
		if err != nil {
			return nil, err
		}
		principals = append(principals, userPrincipals...)
	}

	if principalType == "" || principalType == "group" {
		groupPrincipals, err := p.searchGroup(name, config, lConn)
		if err != nil {
			return nil, err
		}
		principals = append(principals, groupPrincipals...)
	}

	return principals, nil
}

func (p *ldapProvider) searchUser(name string, config *v3.LdapConfig, lConn *ldapv2.Conn) ([]v3.Principal, error) {
	srchAttributes := strings.Split(config.UserSearchAttribute, "|")
	query := fmt.Sprintf("(&(%v=%v)", ObjectClass, config.UserObjectClass)
	srchAttrs := "(|"
	for _, attr := range srchAttributes {
		srchAttrs += fmt.Sprintf("(%v=%v*)", attr, name)
	}
	query += srchAttrs + "))"
	logrus.Debugf("%s searchUser query: %s", p.providerName, query)
	return p.searchLdap(query, p.userScope, config, lConn)
}

func (p *ldapProvider) searchGroup(name string, config *v3.LdapConfig, lConn *ldapv2.Conn) ([]v3.Principal, error) {
	query := fmt.Sprintf("(&(%v=*%v*)(%v=%v))", config.GroupSearchAttribute, name, ObjectClass, config.GroupObjectClass)
	logrus.Debugf("%s searchGroup query: %s", p.providerName, query)
	return p.searchLdap(query, p.groupScope, config, lConn)
}

func (p *ldapProvider) searchLdap(query string, scope string, config *v3.LdapConfig, lConn *ldapv2.Conn) ([]v3.Principal, error) {
	var principals []v3.Principal
	var search *ldapv2.SearchRequest

	entityType := strings.Split(scope, "_")[1]
	searchDomain := config.UserSearchBase
	if strings.EqualFold("user", entityType) {
		search = ldapv2.NewSearchRequest(searchDomain,
			ldapv2.ScopeWholeSubtree, ldapv2.NeverDerefAliases, 0, 0, false,
			query,
			ldap.GetUserSearchAttributesForLDAP(ObjectClass, config), nil)
	} else {
		if config.GroupSearchBase != "" {
			searchDomain = config.GroupSearchBase
		}
		search = ldapv2.NewSearchRequest(searchDomain,
			ldapv2.ScopeWholeSubtree, ldapv2.NeverDerefAliases, 0, 0, false,
			query,
			ldap.GetGroupSearchAttributesForLDAP(ObjectClass, config), nil)
	}

	// Bind before query
	serviceAccountUsername := ldap.GetUserExternalID(config.ServiceAccountDistinguishedName, "")
	err := lConn.Bind(serviceAccountUsername, config.ServiceAccountPassword)
	if err != nil {
		return nil, fmt.Errorf("Error %v in ldap bind", err)
	}

	results, err := lConn.SearchWithPaging(search, 1000)
	if err != nil {
		ldapErr, ok := reflect.ValueOf(err).Interface().(*ldapv2.Error)
		if ok && ldapErr.ResultCode != ldapv2.LDAPResultNoSuchObject {
			return []v3.Principal{}, fmt.Errorf("When searching ldap, Failed to search: %s, error: %#v", query, err)
		}
	}

	for i := 0; i < len(results.Entries); i++ {
		entry := results.Entries[i]
		principal, err := ldap.AttributesToPrincipal(entry.Attributes, results.Entries[i].DN, scope, p.providerName, config.UserObjectClass, config.UserNameAttribute, config.UserLoginAttribute, config.GroupObjectClass, config.GroupNameAttribute)
		if err != nil {
			return []v3.Principal{}, err
		}
		principals = append(principals, *principal)
	}

	return principals, nil
}

func (p *ldapProvider) ldapConnection(config *v3.LdapConfig, caPool *x509.CertPool) (*ldapv2.Conn, error) {
	servers := config.Servers
	TLS := config.TLS
	port := config.Port
	connectionTimeout := config.ConnectionTimeout
	return ldap.NewLDAPConn(servers, TLS, port, connectionTimeout, caPool)
}

func (p *ldapProvider) permissionCheck(attributes []*ldapv2.EntryAttribute, config *v3.LdapConfig) bool {
	userObjectClass := config.UserObjectClass
	userEnabledAttribute := config.UserEnabledAttribute
	userDisabledBitMask := config.UserDisabledBitMask
	return ldap.HasPermission(attributes, userObjectClass, userEnabledAttribute, userDisabledBitMask)
}
