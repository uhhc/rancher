package deployer

import (
	"fmt"
	"reflect"

	"github.com/rancher/norman/controller"
	alertutil "github.com/rancher/rancher/pkg/controllers/user/alert/common"
	"github.com/rancher/rancher/pkg/controllers/user/alert/manager"
	"github.com/rancher/rancher/pkg/controllers/user/helm/common"
	monitorutil "github.com/rancher/rancher/pkg/monitoring"
	"github.com/rancher/rancher/pkg/namespace"
	projectutil "github.com/rancher/rancher/pkg/project"
	"github.com/rancher/rancher/pkg/ref"
	"github.com/rancher/rancher/pkg/settings"
	appsv1beta2 "github.com/rancher/types/apis/apps/v1beta2"
	"github.com/rancher/types/apis/core/v1"
	mgmtv3 "github.com/rancher/types/apis/management.cattle.io/v3"
	projectv3 "github.com/rancher/types/apis/project.cattle.io/v3"
	rbacv1 "github.com/rancher/types/apis/rbac.authorization.k8s.io/v1"
	"github.com/rancher/types/config"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	creatorIDAnn       = "field.cattle.io/creatorId"
	systemProjectLabel = map[string]string{"authz.management.cattle.io/system-project": "true"}
)

type Deployer struct {
	clusterName             string
	alertManager            *manager.AlertManager
	clusterAlertGroupLister mgmtv3.ClusterAlertGroupLister
	projectAlertGroupLister mgmtv3.ProjectAlertGroupLister
	notifierLister          mgmtv3.NotifierLister
	projectLister           mgmtv3.ProjectLister
	clusters                mgmtv3.ClusterInterface
	appDeployer             *appDeployer
	operaterDeployer        *operaterDeployer
}

type appDeployer struct {
	appsGetter       projectv3.AppsGetter
	namespaces       v1.NamespaceInterface
	secrets          v1.SecretInterface
	templateVersions mgmtv3.CatalogTemplateVersionInterface
	statefulsets     appsv1beta2.StatefulSetInterface
}

type operaterDeployer struct {
	templateVersions mgmtv3.CatalogTemplateVersionInterface
	projectsGetter   mgmtv3.ProjectsGetter
	appsGetter       projectv3.AppsGetter
	rbacs            rbacv1.Interface
	cores            v1.Interface
	apps             appsv1beta2.Interface
}

func NewDeployer(cluster *config.UserContext, manager *manager.AlertManager) *Deployer {
	appsgetter := cluster.Management.Project
	ad := &appDeployer{
		appsGetter:       appsgetter,
		namespaces:       cluster.Core.Namespaces(metav1.NamespaceAll),
		secrets:          cluster.Core.Secrets(metav1.NamespaceAll),
		templateVersions: cluster.Management.Management.CatalogTemplateVersions(namespace.GlobalNamespace),
		statefulsets:     cluster.Apps.StatefulSets(metav1.NamespaceAll),
	}

	op := &operaterDeployer{
		templateVersions: cluster.Management.Management.CatalogTemplateVersions(namespace.GlobalNamespace),
		projectsGetter:   cluster.Management.Management,
		appsGetter:       appsgetter,
		rbacs:            cluster.RBAC,
		cores:            cluster.Core,
	}

	return &Deployer{
		clusterName:             cluster.ClusterName,
		alertManager:            manager,
		clusterAlertGroupLister: cluster.Management.Management.ClusterAlertGroups(cluster.ClusterName).Controller().Lister(),
		projectAlertGroupLister: cluster.Management.Management.ProjectAlertGroups(metav1.NamespaceAll).Controller().Lister(),
		notifierLister:          cluster.Management.Management.Notifiers(cluster.ClusterName).Controller().Lister(),
		projectLister:           cluster.Management.Management.Projects(cluster.ClusterName).Controller().Lister(),
		clusters:                cluster.Management.Management.Clusters(metav1.NamespaceAll),
		appDeployer:             ad,
		operaterDeployer:        op,
	}
}

func (d *Deployer) ProjectGroupSync(key string, alert *mgmtv3.ProjectAlertGroup) (runtime.Object, error) {
	return nil, d.sync()
}

func (d *Deployer) ClusterGroupSync(key string, alert *mgmtv3.ClusterAlertGroup) (runtime.Object, error) {
	return nil, d.sync()
}

func (d *Deployer) ProjectRuleSync(key string, alert *mgmtv3.ProjectAlertRule) (runtime.Object, error) {
	return nil, d.sync()
}

func (d *Deployer) ClusterRuleSync(key string, alert *mgmtv3.ClusterAlertRule) (runtime.Object, error) {
	return nil, d.sync()
}

// //deploy or clean up resources(alertmanager deployment, service, namespace) required by alerting.
func (d *Deployer) sync() error {
	appName, appTargetNamespace := monitorutil.ClusterAlertManagerInfo()

	systemProject, err := projectutil.GetSystemProject(d.clusterName, d.projectLister)
	if err != nil {
		return err
	}

	systemProjectCreator := systemProject.Annotations[creatorIDAnn]
	systemProjectID := ref.Ref(systemProject)

	needDeploy, err := d.needDeploy()
	if err != nil {
		return fmt.Errorf("check alertmanager deployment failed, %v", err)
	}

	cluster, err := d.clusters.Get(d.clusterName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get cluster %s failed, %v", d.clusterName, err)
	}
	newCluster := cluster.DeepCopy()
	newCluster.Spec.EnableClusterAlerting = needDeploy

	if needDeploy {
		if !reflect.DeepEqual(cluster, newCluster) {
			_, err = d.clusters.Update(newCluster)
			if err != nil {
				return fmt.Errorf("update cluster %v failed, %v", d.clusterName, err)
			}
		}

		if d.alertManager.IsDeploy, err = d.appDeployer.deploy(appName, appTargetNamespace, systemProjectID, systemProjectCreator); err != nil {
			return fmt.Errorf("deploy alertmanager failed, %v", err)
		}

		return d.appDeployer.isDeploySuccess(newCluster, alertutil.GetAlertManagerDaemonsetName(appName), appTargetNamespace)
	}

	if d.alertManager.IsDeploy, err = d.appDeployer.cleanup(appName, appTargetNamespace, systemProjectID); err != nil {
		return fmt.Errorf("clean up alertmanager failed, %v", err)
	}
	if mgmtv3.ClusterConditionAlertingEnabled.IsTrue(newCluster) {
		mgmtv3.ClusterConditionAlertingEnabled.False(newCluster)
	}

	if !reflect.DeepEqual(cluster, newCluster) {
		_, err = d.clusters.Update(newCluster)
		if err != nil {
			return fmt.Errorf("update cluster %v failed, %v", d.clusterName, err)
		}
	}

	return nil

}

// //only deploy the alertmanager when notifier is configured and alert is using it.
func (d *Deployer) needDeploy() (bool, error) {
	notifiers, err := d.notifierLister.List("", labels.NewSelector())
	if err != nil {
		return false, err
	}

	if len(notifiers) == 0 {
		return false, err
	}

	clusterAlerts, err := d.clusterAlertGroupLister.List("", labels.NewSelector())
	if err != nil {
		return false, err
	}

	for _, alert := range clusterAlerts {
		if len(alert.Spec.Recipients) > 0 {
			return true, nil
		}
	}

	projectAlerts, err := d.projectAlertGroupLister.List("", labels.NewSelector())
	if err != nil {
		return false, nil
	}

	for _, alert := range projectAlerts {
		if controller.ObjectInCluster(d.clusterName, alert) {
			if len(alert.Spec.Recipients) > 0 {
				return true, nil
			}
		}
	}

	return false, nil
}

func (d *appDeployer) isDeploySuccess(cluster *mgmtv3.Cluster, appName, appTargetNamespace string) error {
	_, err := mgmtv3.ClusterConditionAlertingEnabled.DoUntilTrue(cluster, func() (runtime.Object, error) {
		_, err := d.statefulsets.GetNamespaced(appTargetNamespace, appName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get Alertmanager Deployment information, %v", err)
		}

		return cluster, nil
	})
	return err
}

func (d *appDeployer) cleanup(appName, appTargetNamespace, systemProjectID string) (bool, error) {
	_, systemProjectName := ref.Parse(systemProjectID)

	var errgrp errgroup.Group

	errgrp.Go(func() error {
		return d.appsGetter.Apps(systemProjectName).Delete(appName, &metav1.DeleteOptions{})
	})

	errgrp.Go(func() error {
		secretName := alertutil.GetAlertManagerSecretName(appName)
		return d.secrets.DeleteNamespaced(appTargetNamespace, secretName, &metav1.DeleteOptions{})
	})

	if err := errgrp.Wait(); err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}

	return false, nil
}

func (d *appDeployer) getSecret(secretName, secretNamespace string) *corev1.Secret {
	cfg := manager.GetAlertManagerDefaultConfig()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNamespace,
			Name:      secretName,
		},
		Data: map[string][]byte{
			"alertmanager.yaml": data,
			"notification.tmpl": []byte(NotificationTmpl),
		},
	}
}

func (d *appDeployer) deploy(appName, appTargetNamespace, systemProjectID, systemProjectCreator string) (bool, error) {
	_, systemProjectName := ref.Parse(systemProjectID)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: appTargetNamespace,
		},
	}

	if _, err := d.namespaces.Create(ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return false, fmt.Errorf("create ns %s failed, %v", appTargetNamespace, err)
	}

	secretName := alertutil.GetAlertManagerSecretName(appName)
	secret := d.getSecret(secretName, appTargetNamespace)
	if _, err := d.secrets.Create(secret); err != nil && !apierrors.IsAlreadyExists(err) {
		return false, fmt.Errorf("create secret %s:%s failed, %v", appTargetNamespace, appName, err)
	}

	app, err := d.appsGetter.Apps(systemProjectName).Get(appName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("failed to query %q App in %s Project, %v", appName, systemProjectName, err)
	}
	if app.Name == appName {
		if app.DeletionTimestamp != nil {
			return false, fmt.Errorf("stale %q App in %s Project is still on terminating", appName, systemProjectName)
		}
		return true, nil
	}

	catalogID := settings.SystemMonitoringCatalogID.Get()
	templateVersionID, templateVersionNamespace, err := common.ParseExternalID(catalogID)
	if err != nil {
		return false, fmt.Errorf("failed to parse catalog ID %q, %v", catalogID, err)
	}
	if _, err := d.templateVersions.GetNamespaced(templateVersionNamespace, templateVersionID, metav1.GetOptions{}); err != nil {
		return false, fmt.Errorf("failed to find catalog by ID %q, %v", catalogID, err)
	}

	app = &projectv3.App{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				creatorIDAnn: systemProjectCreator,
			},
			Labels:    monitorutil.OwnedLabels(appName, appTargetNamespace, systemProjectID, monitorutil.SystemLevel),
			Name:      appName,
			Namespace: systemProjectName,
		},
		Spec: projectv3.AppSpec{
			Answers: map[string]string{
				"alertmanager.enabled":                "true",
				"alertmanager.serviceMonitor.enabled": "true",
				"alertmanager.apiGroup":               monitorutil.APIVersion.Group,
				"alertmanager.enabledRBAC":            "false",
				"alertmanager.configFromSecret":       secret.Name,
			},
			Description:     "Alertmanager for Rancher Monitoring",
			ExternalID:      catalogID,
			ProjectName:     systemProjectID,
			TargetNamespace: appTargetNamespace,
		},
	}
	if _, err := d.appsGetter.Apps(systemProjectName).Create(app); err != nil && !apierrors.IsAlreadyExists(err) {
		return false, fmt.Errorf("failed to create %q App, %v", appName, err)
	}

	return true, nil
}
