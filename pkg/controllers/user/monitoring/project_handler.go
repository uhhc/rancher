package monitoring

import (
	"fmt"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"github.com/rancher/rancher/pkg/monitoring"
	"github.com/rancher/rancher/pkg/ref"
	"github.com/rancher/rancher/pkg/settings"
	mgmtv3 "github.com/rancher/types/apis/management.cattle.io/v3"
	v3 "github.com/rancher/types/apis/project.cattle.io/v3"
	"github.com/sirupsen/logrus"
	k8scorev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

const (
	prtbBySA = "monitoring.project.cattle.io/prtb-by-sa"
)

type projectHandler struct {
	clusterName         string
	cattleClusterClient mgmtv3.ClusterInterface
	cattleProjectClient mgmtv3.ProjectInterface
	prtbClient          mgmtv3.ProjectRoleTemplateBindingInterface
	prtbIndexer         cache.Indexer
	app                 *appHandler
}

func (ph *projectHandler) sync(key string, project *mgmtv3.Project) (runtime.Object, error) {
	if project == nil || project.DeletionTimestamp != nil ||
		project.Spec.ClusterName != ph.clusterName {
		return project, nil
	}

	if !mgmtv3.NamespaceBackedResource.IsTrue(project) || !mgmtv3.ProjectConditionInitialRolesPopulated.IsTrue(project) {
		return project, nil
	}

	clusterID := project.Spec.ClusterName
	cluster, err := ph.cattleClusterClient.Get(clusterID, metav1.GetOptions{})
	if err != nil {
		return project, errors.Wrapf(err, "failed to find Cluster %s", clusterID)
	}

	clusterName := cluster.Spec.DisplayName
	projectTag := getProjectTag(project, clusterName)
	src := project
	cpy := src.DeepCopy()

	if err := ph.doProjectGraphsSync(cpy); err != nil {
		return project, errors.Wrapf(err, "failed to add project graph to project %s", projectTag)
	}

	if err := ph.doSync(cpy, clusterName); err != nil {
		return project, errors.Wrapf(err, "failed to sync project %s", projectTag)
	}

	if !reflect.DeepEqual(cpy, src) {
		updated, err := ph.cattleProjectClient.Update(cpy)
		if err != nil {
			return project, errors.Wrapf(err, "failed to update project %s", projectTag)
		}

		cpy = updated
	}

	return cpy, err
}

func (ph *projectHandler) doSync(project *mgmtv3.Project, clusterName string) error {
	appName, appTargetNamespace := monitoring.ProjectMonitoringInfo(project.Name)
	var err error
	if project.Spec.EnableProjectMonitoring {
		_, err = mgmtv3.ProjectConditionMonitoringEnabled.Do(project, func() (runtime.Object, error) {
			appProjectName, err := ph.ensureAppProjectName(appTargetNamespace, project)
			if err != nil {
				return project, errors.Wrap(err, "failed to ensure monitoring project name")
			}
			if err := ph.deployApp(appName, appTargetNamespace, appProjectName, project, clusterName); err != nil {
				return project, errors.Wrap(err, "failed to deploy monitoring")
			}
			if err := ph.detectAppComponentsWhileInstall(appName, appTargetNamespace, project); err != nil {
				return project, errors.Wrap(err, "failed to detect the installation status of monitoring components")
			}
			return project, nil
		})
		// set unknown when error to keep consistency as before. TODO, we should add a new condition for monitoring disabled
		if err != nil {
			mgmtv3.ProjectConditionMonitoringEnabled.Unknown(project)
		}
	} else if enabledStatus := mgmtv3.ProjectConditionMonitoringEnabled.GetStatus(project); enabledStatus != "" && enabledStatus != "False" {
		err = func() error {
			if err := ph.app.withdrawApp(project.Spec.ClusterName, appName, appTargetNamespace); err != nil {
				return errors.Wrap(err, "failed to withdraw monitoring")
			}

			if err := ph.detectAppComponentsWhileUninstall(appName, appTargetNamespace, project); err != nil {
				return errors.Wrap(err, "failed to detect the uninstallation status of monitoring components")
			}
			return nil
		}()

		if err == nil {
			mgmtv3.ProjectConditionMonitoringEnabled.False(project)
			mgmtv3.ProjectConditionMonitoringEnabled.Message(project, "")
		} else {
			mgmtv3.ProjectConditionMonitoringEnabled.Unknown(project)
			mgmtv3.ProjectConditionMonitoringEnabled.Message(project, err.Error())
		}
	}

	return nil
}

// doProjectGraphsSync creates the metric graphs per project for cluster monitoring
func (ph *projectHandler) doProjectGraphsSync(project *mgmtv3.Project) error {
	_, err := mgmtv3.ProjectConditionMetricExpressionDeployed.DoUntilTrue(project, func() (runtime.Object, error) {
		projectName := ref.FromStrings(project.Namespace, project.Name)

		for _, graph := range preDefinedProjectGraph {
			newObj := graph.DeepCopy()
			newObj.Namespace = project.Name
			newObj.Spec.ProjectName = projectName
			if _, err := ph.app.cattleProjectGraphClient.Create(newObj); err != nil && !apierrors.IsAlreadyExists(err) {
				return project, err
			}
		}

		return project, nil
	})
	// ignore error here becuase we set error in condition
	if err != nil {
		logrus.Errorf("failed to apply metric expression,err: %s", err.Error())
	}

	return nil
}

func (ph *projectHandler) ensureAppProjectName(appTargetNamespace string, project *mgmtv3.Project) (string, error) {
	appProjectName, err := monitoring.EnsureAppProjectName(ph.app.agentNamespaceClient, project.Name, project.Spec.ClusterName, appTargetNamespace)
	if err != nil {
		return "", err
	}

	return appProjectName, nil
}

func (ph *projectHandler) deployApp(appName, appTargetNamespace string, appProjectName string, project *mgmtv3.Project, clusterName string) error {
	appDeployProjectID := project.Name
	clusterPrometheusSvcName, clusterPrometheusSvcNamespaces, clusterPrometheusPort := monitoring.ClusterPrometheusEndpoint()
	clusterAlertManagerSvcName, clusterAlertManagerSvcNamespaces, clusterAlertManagerPort := monitoring.ClusterAlertManagerEndpoint()
	optionalAppAnswers := map[string]string{
		"grafana.persistence.enabled": "false",

		"prometheus.persistence.enabled": "false",

		"prometheus.sync.mode": "federate",
	}

	mustAppAnswers := map[string]string{
		"operator.enabled": "false",

		"exporter-coredns.enabled": "false",

		"exporter-kube-controller-manager.enabled": "false",

		"exporter-kube-dns.enabled": "false",

		"exporter-kube-scheduler.enabled": "false",

		"exporter-kube-state.enabled": "false",

		"exporter-kubelets.enabled": "false",

		"exporter-kubernetes.enabled": "false",

		"exporter-node.enabled": "false",

		"exporter-fluentd.enabled": "false",

		"grafana.enabled":  "true",
		"grafana.level":    "project",
		"grafana.apiGroup": monitoring.APIVersion.Group,

		"prometheus.enabled":                       "true",
		"prometheus.level":                         "project",
		"prometheus.apiGroup":                      monitoring.APIVersion.Group,
		"prometheus.serviceAccountNameOverride":    appName,
		"prometheus.project.alertManagerTarget":    fmt.Sprintf("%s.%s:%s", clusterAlertManagerSvcName, clusterAlertManagerSvcNamespaces, clusterAlertManagerPort),
		"prometheus.project.projectDisplayName":    project.Spec.DisplayName,
		"prometheus.project.clusterDisplayName":    clusterName,
		"prometheus.cluster.alertManagerNamespace": clusterAlertManagerSvcNamespaces,
	}

	appAnswers := monitoring.OverwriteAppAnswers(optionalAppAnswers, project.Annotations)

	// cannot overwrite mustAppAnswers
	for mustKey, mustVal := range mustAppAnswers {
		appAnswers[mustKey] = mustVal
	}

	// complete sync target & path
	if appAnswers["prometheus.sync.mode"] == "federate" {
		appAnswers["prometheus.sync.target"] = fmt.Sprintf("%s.%s:%s", clusterPrometheusSvcName, clusterPrometheusSvcNamespaces, clusterPrometheusPort)
		appAnswers["prometheus.sync.path"] = "/federate"
	} else {
		appAnswers["prometheus.sync.target"] = fmt.Sprintf("http://%s.%s:%s", clusterPrometheusSvcName, clusterPrometheusSvcNamespaces, clusterPrometheusPort)
		appAnswers["prometheus.sync.path"] = "/api/v1/read"
	}

	creator, err := ph.app.systemAccountManager.GetProjectSystemUser(project.Name)
	if err != nil {
		return err
	}

	appCatalogID := settings.SystemMonitoringCatalogID.Get()
	app := &v3.App{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{creatorIDAnno: creator.Name},
			Labels:      monitoring.OwnedLabels(appName, appTargetNamespace, appProjectName, monitoring.ProjectLevel),
			Name:        appName,
			Namespace:   appDeployProjectID,
		},
		Spec: v3.AppSpec{
			Answers:         appAnswers,
			Description:     "Rancher Project Monitoring",
			ExternalID:      appCatalogID,
			ProjectName:     appProjectName,
			TargetNamespace: appTargetNamespace,
		},
	}

	deployed, err := monitoring.DeployApp(ph.app.cattleAppClient, appDeployProjectID, app, false)
	if err != nil {
		return err
	}

	return ph.deployAppRoles(deployed)
}

func (ph *projectHandler) detectAppComponentsWhileInstall(appName, appTargetNamespace string, project *mgmtv3.Project) error {
	if project.Status.MonitoringStatus == nil {
		project.Status.MonitoringStatus = &mgmtv3.MonitoringStatus{
			Conditions: []mgmtv3.MonitoringCondition{
				{Type: mgmtv3.ClusterConditionType(ConditionPrometheusDeployed), Status: k8scorev1.ConditionFalse},
				{Type: mgmtv3.ClusterConditionType(ConditionGrafanaDeployed), Status: k8scorev1.ConditionFalse},
			},
		}
	}
	monitoringStatus := project.Status.MonitoringStatus

	checkers := make([]func() error, 0, len(monitoringStatus.Conditions))
	if !ConditionGrafanaDeployed.IsTrue(monitoringStatus) {
		checkers = append(checkers, func() error {
			return isGrafanaDeployed(ph.app.agentDeploymentClient, appTargetNamespace, appName, monitoringStatus, project.Spec.ClusterName)
		})
	}
	if !ConditionPrometheusDeployed.IsTrue(monitoringStatus) {
		checkers = append(checkers, func() error {
			return isPrometheusDeployed(ph.app.agentStatefulSetClient, appTargetNamespace, appName, monitoringStatus)
		})
	}

	if len(checkers) == 0 {
		return nil
	}

	err := stream(checkers...)
	if err != nil {
		time.Sleep(5 * time.Second)
	}

	return err
}

func (ph *projectHandler) detectAppComponentsWhileUninstall(appName, appTargetNamespace string, project *mgmtv3.Project) error {
	if project.Status.MonitoringStatus == nil {
		return nil
	}
	monitoringStatus := project.Status.MonitoringStatus

	checkers := make([]func() error, 0, len(monitoringStatus.Conditions))
	if !ConditionGrafanaDeployed.IsFalse(monitoringStatus) {
		checkers = append(checkers, func() error {
			return isGrafanaWithdrew(ph.app.agentDeploymentClient, appTargetNamespace, appName, monitoringStatus)
		})
	}
	if !ConditionPrometheusDeployed.IsFalse(monitoringStatus) {
		checkers = append(checkers, func() error {
			return isPrometheusWithdrew(ph.app.agentStatefulSetClient, appTargetNamespace, appName, monitoringStatus)
		})
	}

	if len(checkers) == 0 {
		return nil
	}

	err := stream(checkers...)
	if err != nil {
		time.Sleep(5 * time.Second)
	}

	return err
}

func getProjectTag(project *mgmtv3.Project, clusterName string) string {
	return fmt.Sprintf("%s(%s) of Cluster %s(%s)", project.Name, project.Spec.DisplayName, project.Spec.ClusterName, clusterName)
}

func (ph *projectHandler) deployAppRoles(app *v3.App) error {
	if app.DeletionTimestamp != nil {
		return nil
	}

	namespace, name := app.Spec.TargetNamespace, app.Name
	objects, err := ph.prtbIndexer.ByIndex(prtbBySA, fmt.Sprintf("%s:%s", namespace, name))
	if err != nil {
		return err
	}
	if len(objects) != 0 {
		return nil
	}

	controller := true
	reference := metav1.OwnerReference{
		APIVersion: app.APIVersion,
		Kind:       app.Kind,
		Name:       app.Name,
		UID:        app.UID,
		Controller: &controller,
	}

	_, err = ph.prtbClient.Create(&mgmtv3.ProjectRoleTemplateBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:    "app-",
			Namespace:       app.Namespace,
			OwnerReferences: []metav1.OwnerReference{reference},
		},
		ServiceAccount:   fmt.Sprintf("%s:%s", app.Spec.TargetNamespace, app.Name),
		ProjectName:      ref.FromStrings(ph.clusterName, app.Namespace),
		RoleTemplateName: "project-monitoring-readonly",
	})

	return err
}

func prtbBySAFunc(obj interface{}) ([]string, error) {
	projectRoleBinding, ok := obj.(*mgmtv3.ProjectRoleTemplateBinding)
	if !ok || projectRoleBinding.ServiceAccount == "" {
		return []string{}, nil
	}

	return []string{projectRoleBinding.ServiceAccount}, nil
}
