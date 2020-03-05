package k3supgrade

import (
	"fmt"
	"strings"

	"github.com/coreos/go-semver/semver"
	utils2 "github.com/rancher/rancher/pkg/app/utils"
	"github.com/rancher/rancher/pkg/catalog/utils"
	"github.com/rancher/rancher/pkg/namespace"
	"github.com/rancher/rancher/pkg/project"
	"github.com/rancher/rancher/pkg/ref"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	v32 "github.com/rancher/types/apis/project.cattle.io/v3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (h *handler) onClusterChange(key string, cluster *v3.Cluster) (*v3.Cluster, error) {
	if cluster == nil || cluster.DeletionTimestamp != nil {
		return nil, nil
	}

	// only applies to k3s clusters
	if cluster.Status.Driver != v3.ClusterDriverK3s {
		return cluster, nil
	}

	// setup default k3sconfig
	if cluster.Spec.K3sConfig == nil {
		version := *cluster.Status.Version
		cluster.Spec.K3sConfig = &v3.K3sConfig{
			Version: version.String(),
		}
	}

	if cluster.Spec.K3sConfig.Version == "" {
		cluster.Spec.K3sConfig.Version = cluster.Status.Version.String()
	}

	isNewer, err := IsNewerVersion(cluster.Status.Version.GitVersion, cluster.Spec.K3sConfig.Version)
	if err != nil {
		return cluster, err
	}

	if !isNewer {
		// cannot upgrade- version has not changed or proposed version is not newer than current version
		return cluster, nil
	}

	// create or update k3supgradecontroller if necessary
	if err = h.deployK3sUpgradeController(cluster.Name); err != nil {
		return cluster, err
	}

	// deploy plans into downstream cluster
	if err = h.deployPlans(cluster); err != nil {
		return cluster, err
	}

	return cluster, nil
}

// deployK3sUpgradeController creates a rancher k3s upgrader controller if one does not exist.
// Updates k3s upgrader controller if one exists and is not the newest available version.
func (h *handler) deployK3sUpgradeController(clusterName string) error {
	userCtx, err := h.manager.UserContext(clusterName)
	if err != nil {
		return err
	}

	projectLister := userCtx.Management.Management.Projects("").Controller().Lister()
	systemProject, err := project.GetSystemProject(clusterName, projectLister)
	if err != nil {
		return err
	}

	templateID := k3sUpgraderCatalogName
	template, err := h.templateLister.Get(namespace.GlobalNamespace, templateID)
	if err != nil {
		return err
	}

	latestTemplateVersion, err := utils.LatestAvailableTemplateVersion(template)
	if err != nil {
		return err
	}

	creator, err := h.systemAccountManager.GetSystemUser(clusterName)
	if err != nil {
		return err
	}
	systemProjectID := ref.Ref(systemProject)
	_, systemProjectName := ref.Parse(systemProjectID)

	nsClient := userCtx.Core.Namespaces("")
	appProjectName, err := utils2.EnsureAppProjectName(nsClient, systemProjectName, clusterName, systemUpgradeNS, creator.Name)
	if err != nil {
		return err
	}

	appLister := userCtx.Management.Project.Apps("").Controller().Lister()
	appClient := userCtx.Management.Project.Apps("")

	latestVersionID := latestTemplateVersion.ExternalID

	app, err := appLister.Get(systemProjectName, "rancher-k3s-upgrader")
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		desiredApp := &v32.App{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rancher-k3s-upgrader",
				Namespace: systemProjectName,
				Annotations: map[string]string{
					"field.cattle.io/creatorId": creator.Name,
				},
			},
			Spec: v32.AppSpec{
				Description:     "Upgrade controller for k3s clusters",
				ExternalID:      latestVersionID,
				ProjectName:     appProjectName,
				TargetNamespace: systemUpgradeNS,
			},
		}

		// k3s upgrader doesn't exist yet, so it will need to be created
		if _, err = appClient.Create(desiredApp); err != nil {
			return err
		}
	} else {
		if app.Spec.ExternalID == latestVersionID {
			// app is up to date, no action needed
			return nil
		}
		desiredApp := app.DeepCopy()
		desiredApp.Spec.ExternalID = latestVersionID
		// new version of k3s upgrade available, update app
		if _, err = appClient.Update(desiredApp); err != nil {
			return err
		}
	}

	return nil
}

// isNewerVersion returns true if updated versions semver is newer and false if its
// semver is older. If semver is equal then metadata is alphanumerically compared.
func IsNewerVersion(prevVersion, updatedVersion string) (bool, error) {
	parseErrMsg := "failed to parse version: %v"
	prevVer, err := semver.NewVersion(strings.TrimPrefix(prevVersion, "v"))
	if err != nil {
		return false, fmt.Errorf(parseErrMsg, err)
	}

	updatedVer, err := semver.NewVersion(strings.TrimPrefix(updatedVersion, "v"))
	if err != nil {
		return false, fmt.Errorf(parseErrMsg, err)
	}

	switch updatedVer.Compare(*prevVer) {
	case -1:
		return false, nil
	case 1:
		return true, nil
	default:
		// using metadata to determine precedence is against semver standards
		// this is ignored because it because k3s uses it to precedence between
		// two versions based on same k8s version
		return updatedVer.Metadata > prevVer.Metadata, nil
	}
}
