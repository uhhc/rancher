package cluster

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rancher/norman/api/access"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/norman/types/convert"
	gaccess "github.com/rancher/rancher/pkg/api/customization/globalnamespaceaccess"
	"github.com/rancher/rancher/pkg/controllers/management/k3supgrade"
	"github.com/rancher/rancher/pkg/namespace"
	"github.com/rancher/rancher/pkg/settings"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	mgmtSchema "github.com/rancher/types/apis/management.cattle.io/v3/schema"
	mgmtclient "github.com/rancher/types/client/management/v3"
)

type Validator struct {
	ClusterLister                 v3.ClusterLister
	ClusterTemplateLister         v3.ClusterTemplateLister
	ClusterTemplateRevisionLister v3.ClusterTemplateRevisionLister
	Users                         v3.UserInterface
	GrbLister                     v3.GlobalRoleBindingLister
	GrLister                      v3.GlobalRoleLister
}

func (v *Validator) Validator(request *types.APIContext, schema *types.Schema, data map[string]interface{}) error {
	var spec v3.ClusterSpec

	if err := convert.ToObj(data, &spec); err != nil {
		return httperror.WrapAPIError(err, httperror.InvalidBodyContent, "Cluster spec conversion error")
	}

	if err := v.validateEnforcement(request, data); err != nil {
		return err
	}

	if err := v.validateLocalClusterAuthEndpoint(request, &spec); err != nil {
		return err
	}

	if err := v.validateK3sVersionUpgrade(request, &spec); err != nil {
		return err
	}

	return nil
}

func (v *Validator) validateLocalClusterAuthEndpoint(request *types.APIContext, spec *v3.ClusterSpec) error {
	if !spec.LocalClusterAuthEndpoint.Enabled {
		return nil
	}

	var isValidCluster bool
	if request.ID == "" {
		isValidCluster = spec.RancherKubernetesEngineConfig != nil
	} else {
		cluster, err := v.ClusterLister.Get("", request.ID)
		if err != nil {
			return err
		}
		isValidCluster = cluster.Status.Driver == "" ||
			cluster.Status.Driver == v3.ClusterDriverRKE ||
			cluster.Status.Driver == v3.ClusterDriverImported
	}
	if !isValidCluster {
		return httperror.NewFieldAPIError(httperror.InvalidState, "LocalClusterAuthEndpoint.Enabled", "Can only enable LocalClusterAuthEndpoint with RKE")
	}

	if spec.LocalClusterAuthEndpoint.CACerts != "" && spec.LocalClusterAuthEndpoint.FQDN == "" {
		return httperror.NewFieldAPIError(httperror.MissingRequired, "LocalClusterAuthEndpoint.FQDN", "CACerts defined but FQDN is not defined")
	}

	return nil
}

func (v *Validator) validateEnforcement(request *types.APIContext, data map[string]interface{}) error {

	if !strings.EqualFold(settings.ClusterTemplateEnforcement.Get(), "true") {
		return nil
	}

	var spec mgmtclient.Cluster
	if err := convert.ToObj(data, &spec); err != nil {
		return httperror.WrapAPIError(err, httperror.InvalidBodyContent, "Cluster spec conversion error")
	}

	if !v.checkClusterForEnforcement(&spec) {
		return nil
	}

	ma := gaccess.MemberAccess{
		Users:     v.Users,
		GrLister:  v.GrLister,
		GrbLister: v.GrbLister,
	}

	//if user is admin, no checks needed
	callerID := request.Request.Header.Get(gaccess.ImpersonateUserHeader)

	isAdmin, err := ma.IsAdmin(callerID)
	if err != nil {
		return err
	}
	if isAdmin {
		return nil
	}

	//enforcement is true, template is a must
	if spec.ClusterTemplateRevisionID == "" {
		return httperror.NewFieldAPIError(httperror.MissingRequired, "", "A clusterTemplateRevision to create a cluster")
	}

	err = v.accessTemplate(request, &spec)
	if err != nil {
		if httperror.IsForbidden(err) || httperror.IsNotFound(err) {
			return httperror.NewAPIError(httperror.NotFound, "The clusterTemplateRevision is not found")
		}
		return err
	}

	return nil
}

// TODO: test validator
// prevents downgrades, no-ops, and upgrading before versions have been set
func (v *Validator) validateK3sVersionUpgrade(request *types.APIContext, spec *v3.ClusterSpec) error {
	upgradeNotReadyErr := httperror.NewAPIError(httperror.Conflict, "k3s version upgrade is not ready, try again later")

	if request.Method == http.MethodPost {
		return nil
	}

	if spec.K3sConfig == nil {
		// only applies to k3s clusters
		return nil
	}

	// must wait for original spec version to be set
	if spec.K3sConfig.Version == "" {
		return upgradeNotReadyErr
	}

	cluster, err := v.ClusterLister.Get("", request.ID)
	if err != nil {
		return err
	}

	// must wait for original status version to be set
	if cluster.Status.Version == nil {
		return upgradeNotReadyErr
	}

	prevVersion := cluster.Status.Version.GitVersion
	updateVersion := spec.K3sConfig.Version

	if prevVersion == updateVersion {
		// no op
		return nil
	}

	isNewer, err := k3supgrade.IsNewerVersion(prevVersion, updateVersion)
	if err != nil {
		errMsg := fmt.Sprintf("unable to compare k3s version [%s]", spec.K3sConfig.Version)
		return httperror.NewAPIError(httperror.InvalidBodyContent, errMsg)
	}

	if !isNewer {
		// update version must be higher than previous version, downgrades are not supported
		errMsg := fmt.Sprintf("cannot upgrade k3s cluster version from [%s] to [%s]. New version must be higher.", prevVersion, updateVersion)
		return httperror.NewAPIError(httperror.InvalidBodyContent, errMsg)
	}

	return nil
}

func (v *Validator) checkClusterForEnforcement(spec *mgmtclient.Cluster) bool {
	if spec.RancherKubernetesEngineConfig != nil {
		return true
	}

	if spec.ClusterTemplateRevisionID != "" {
		return true
	}
	return false
}

func (v *Validator) accessTemplate(request *types.APIContext, spec *mgmtclient.Cluster) error {
	split := strings.SplitN(spec.ClusterTemplateRevisionID, ":", 2)
	if len(split) != 2 {
		return fmt.Errorf("error in splitting clusterTemplateRevision name %v", spec.ClusterTemplateRevisionID)
	}
	revName := split[1]
	clusterTempRev, err := v.ClusterTemplateRevisionLister.Get(namespace.GlobalNamespace, revName)
	if err != nil {
		return err
	}

	var ctMap map[string]interface{}
	if err := access.ByID(request, &mgmtSchema.Version, mgmtclient.ClusterTemplateType, clusterTempRev.Spec.ClusterTemplateName, &ctMap); err != nil {
		return err
	}

	return nil
}
