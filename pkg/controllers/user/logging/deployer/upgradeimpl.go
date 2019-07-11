package deployer

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/rancher/rancher/pkg/controllers/user/helm/common"
	loggingconfig "github.com/rancher/rancher/pkg/controllers/user/logging/config"
	"github.com/rancher/rancher/pkg/project"
	"github.com/rancher/rancher/pkg/settings"
	appsv1beta2 "github.com/rancher/types/apis/apps/v1beta2"
	v1 "github.com/rancher/types/apis/core/v1"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"

	"github.com/pkg/errors"
	"github.com/rancher/rancher/pkg/namespace"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var (
	ServiceName             = "logging"
	waitCatalogSyncInterval = 60 * time.Second
)

type LoggingService struct {
	clusterName    string
	clusterLister  v3.ClusterLister
	catalogLister  v3.CatalogLister
	projectLister  v3.ProjectLister
	templateLister v3.CatalogTemplateLister
	daemonsets     appsv1beta2.DaemonSetInterface
	secrets        v1.SecretInterface
	appDeployer    *AppDeployer
}

func NewService() *LoggingService {
	return &LoggingService{}
}

func (l *LoggingService) Init(cluster *config.UserContext) {
	ad := &AppDeployer{
		AppsGetter: cluster.Management.Project,
		Namespaces: cluster.Core.Namespaces(metav1.NamespaceAll),
	}

	l.clusterName = cluster.ClusterName
	l.clusterLister = cluster.Management.Management.Clusters("").Controller().Lister()
	l.catalogLister = cluster.Management.Management.Catalogs(metav1.NamespaceAll).Controller().Lister()
	l.projectLister = cluster.Management.Management.Projects(cluster.ClusterName).Controller().Lister()
	l.templateLister = cluster.Management.Management.CatalogTemplates(metav1.NamespaceAll).Controller().Lister()
	l.daemonsets = cluster.Apps.DaemonSets(loggingconfig.LoggingNamespace)
	l.secrets = cluster.Core.Secrets(loggingconfig.LoggingNamespace)
	l.appDeployer = ad
}

func (l *LoggingService) Version() (string, error) {
	catalogID := settings.SystemLoggingCatalogID.Get()
	templateVersionID, _, err := common.ParseExternalID(catalogID)
	if err != nil {
		return "", fmt.Errorf("get system logging catalog version failed, %v", err)
	}
	return templateVersionID, nil
}

func (l *LoggingService) Upgrade(currentVersion string) (string, error) {
	appName := loggingconfig.AppName
	templateID := loggingconfig.RancherLoggingTemplateID()
	template, err := l.templateLister.Get(namespace.GlobalNamespace, templateID)
	if err != nil {
		return "", errors.Wrapf(err, "get template %s failed", templateID)
	}

	newFullVersion := fmt.Sprintf("%s-%s", templateID, template.Spec.DefaultVersion)
	if currentVersion == newFullVersion {
		return currentVersion, nil
	}

	// check cluster ready before upgrade, because helm will not retry if got cluster not ready error
	cluster, err := l.clusterLister.Get(metav1.NamespaceAll, l.clusterName)
	if err != nil {
		return "", fmt.Errorf("get cluster %s failed, %v", l.clusterName, err)
	}
	if !v3.ClusterConditionReady.IsTrue(cluster) {
		return "", fmt.Errorf("cluster %v not ready", l.clusterName)
	}

	//clean old version
	if !strings.Contains(currentVersion, templateID) {
		if err = l.removeLegacy(); err != nil {
			return "", err
		}
	}

	//upgrade old app
	newVersion := template.Spec.DefaultVersion
	newCatalogID := loggingconfig.RancherLoggingCatalogID(newVersion)
	defaultSystemProjects, err := l.projectLister.List(metav1.NamespaceAll, labels.Set(project.SystemProjectLabel).AsSelector())
	if err != nil {
		return "", errors.Wrap(err, "list system project failed")
	}

	if len(defaultSystemProjects) == 0 {
		return "", errors.New("get system project failed")
	}

	systemProject := defaultSystemProjects[0]
	if systemProject == nil {
		return "", errors.New("get system project failed")
	}

	app, err := l.appDeployer.AppsGetter.Apps(metav1.NamespaceAll).GetNamespaced(systemProject.Name, appName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return newFullVersion, nil
		}
		return "", errors.Wrapf(err, "get app %s:%s failed", systemProject.Name, appName)
	}

	_, systemCatalogName, _, _, _, err := common.SplitExternalID(newCatalogID)
	if err != nil {
		return "", err
	}

	systemCatalog, err := l.catalogLister.Get(metav1.NamespaceAll, systemCatalogName)
	if err != nil {
		return "", fmt.Errorf("get catalog %s failed, %v", systemCatalogName, err)
	}

	if !v3.CatalogConditionUpgraded.IsTrue(systemCatalog) || !v3.CatalogConditionRefreshed.IsTrue(systemCatalog) || !v3.CatalogConditionDiskCached.IsTrue(systemCatalog) {
		return "", fmt.Errorf("catalog %v not ready", systemCatalogName)
	}

	newApp := app.DeepCopy()
	newApp.Spec.ExternalID = newCatalogID

	if !reflect.DeepEqual(newApp, app) {
		if _, err = l.appDeployer.AppsGetter.Apps(metav1.NamespaceAll).Update(newApp); err != nil {
			return "", errors.Wrapf(err, "update app %s:%s failed", app.Namespace, app.Name)
		}
	}
	return newFullVersion, nil
}

func (l *LoggingService) removeLegacy() error {
	op := metav1.DeletePropagationBackground
	errMsg := "failed to remove legacy logging %s %s:%s when upgrade"

	if err := l.daemonsets.Delete(loggingconfig.FluentdName, &metav1.DeleteOptions{PropagationPolicy: &op}); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, errMsg, loggingconfig.LoggingNamespace, "daemonset", loggingconfig.FluentdName)
	}

	if err := l.daemonsets.Delete(loggingconfig.LogAggregatorName, &metav1.DeleteOptions{PropagationPolicy: &op}); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, errMsg, loggingconfig.LoggingNamespace, "daemonset", loggingconfig.LogAggregatorName)
	}

	legacySSlConfigName := "sslconfig"
	legacyClusterConfigName := "cluster-logging"
	legacyProjectConfigName := "project-logging"

	if err := l.secrets.Delete(legacySSlConfigName, &metav1.DeleteOptions{PropagationPolicy: &op}); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, errMsg, "serect", loggingconfig.LoggingNamespace, legacySSlConfigName)
	}

	if err := l.secrets.Delete(legacyClusterConfigName, &metav1.DeleteOptions{PropagationPolicy: &op}); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, errMsg, "serect", loggingconfig.LoggingNamespace, legacyClusterConfigName)
	}

	if err := l.secrets.Delete(legacyProjectConfigName, &metav1.DeleteOptions{PropagationPolicy: &op}); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, errMsg, "serect", loggingconfig.LoggingNamespace, legacyProjectConfigName)
	}
	return nil
}
