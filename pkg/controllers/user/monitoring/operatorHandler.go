package monitoring

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/rancher/rancher/pkg/monitoring"
	"github.com/rancher/rancher/pkg/settings"
	mgmtv3 "github.com/rancher/types/apis/management.cattle.io/v3"
	projectv3 "github.com/rancher/types/apis/project.cattle.io/v3"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type operatorHandler struct {
	clusterName         string
	cattleClusterClient mgmtv3.ClusterInterface
	app                 *appHandler
}

func (h *operatorHandler) syncCluster(key string, obj *mgmtv3.Cluster) (runtime.Object, error) {
	if obj == nil || obj.DeletionTimestamp != nil || obj.Name != h.clusterName {
		return obj, nil
	}

	if !mgmtv3.ClusterConditionAgentDeployed.IsTrue(obj) {
		return obj, nil
	}

	var newCluster *mgmtv3.Cluster
	var err error
	//should deploy
	if obj.Spec.EnableClusterAlerting || obj.Spec.EnableClusterMonitoring {
		newObj, err := mgmtv3.ClusterConditionPrometheusOperatorDeployed.Do(obj, func() (runtime.Object, error) {
			cpy := obj.DeepCopy()
			return cpy, deploySystemMonitor(cpy, h.app)
		})
		if err != nil {
			logrus.Warnf("deploy prometheus operator error, %v", err)
		}
		newCluster = newObj.(*mgmtv3.Cluster)
	} else { // should withdraw
		newCluster = obj.DeepCopy()
		if err = withdrawSystemMonitor(newCluster, h.app); err != nil {
			logrus.Warnf("withdraw prometheus operator error, %v", err)
		}
	}

	if newCluster != nil && !reflect.DeepEqual(newCluster, obj) {
		if newCluster, err = h.cattleClusterClient.Update(newCluster); err != nil {
			return nil, err
		}
		return newCluster, nil
	}
	return obj, nil
}

func (h *operatorHandler) syncProject(key string, project *mgmtv3.Project) (runtime.Object, error) {
	if project == nil || project.DeletionTimestamp != nil || project.Spec.ClusterName != h.clusterName {
		return project, nil
	}

	clusterID := project.Spec.ClusterName
	cluster, err := h.cattleClusterClient.Get(clusterID, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find Cluster %s", clusterID)
	}

	if !mgmtv3.ClusterConditionAgentDeployed.IsTrue(cluster) {
		return project, nil
	}

	var newCluster *mgmtv3.Cluster
	//should deploy
	if cluster.Spec.EnableClusterAlerting || project.Spec.EnableProjectMonitoring {
		newObj, err := mgmtv3.ClusterConditionPrometheusOperatorDeployed.Do(cluster, func() (runtime.Object, error) {
			cpy := cluster.DeepCopy()
			return cpy, deploySystemMonitor(cpy, h.app)
		})
		if err != nil {
			logrus.Warnf("deploy prometheus operator error, %v", err)
		}
		newCluster = newObj.(*mgmtv3.Cluster)
	} else { // should withdraw
		newCluster = cluster.DeepCopy()
		if err = withdrawSystemMonitor(newCluster, h.app); err != nil {
			logrus.Warnf("withdraw prometheus operator error, %v", err)
		}
	}

	if newCluster != nil && !reflect.DeepEqual(newCluster, cluster) {
		if _, err = h.cattleClusterClient.Update(newCluster); err != nil {
			return nil, err
		}
	}

	return project, nil
}

func withdrawSystemMonitor(cluster *mgmtv3.Cluster, app *appHandler) error {
	isAlertingDisabling := mgmtv3.ClusterConditionAlertingEnabled.IsFalse(cluster) ||
		mgmtv3.ClusterConditionAlertingEnabled.GetStatus(cluster) == ""
	isClusterMonitoringDisabling := mgmtv3.ClusterConditionMonitoringEnabled.IsFalse(cluster) ||
		mgmtv3.ClusterConditionMonitoringEnabled.GetStatus(cluster) == ""
	//status false and empty should withdraw. when status unknown, it means the deployment has error while deploying apps
	isOperatorDeploying := !mgmtv3.ClusterConditionPrometheusOperatorDeployed.IsFalse(cluster)
	areAllOwnedProjectMonitoringDisabling, err := allOwnedProjectsMonitoringDisabling(app.cattleProjectClient)
	if err != nil {
		mgmtv3.ClusterConditionPrometheusOperatorDeployed.Unknown(cluster)
		mgmtv3.ClusterConditionPrometheusOperatorDeployed.ReasonAndMessageFromError(cluster, errors.Wrap(err, "failed to list owned projects of cluster"))
		return err
	}

	if areAllOwnedProjectMonitoringDisabling && isAlertingDisabling && isClusterMonitoringDisabling && isOperatorDeploying {
		appName, appTargetNamespace := monitoring.SystemMonitoringInfo()

		if err := monitoring.WithdrawApp(app.cattleAppClient, monitoring.OwnedAppListOptions(cluster.Name, appName, appTargetNamespace)); err != nil {
			mgmtv3.ClusterConditionPrometheusOperatorDeployed.Unknown(cluster)
			mgmtv3.ClusterConditionPrometheusOperatorDeployed.ReasonAndMessageFromError(cluster, errors.Wrap(err, "failed to withdraw prometheus operator app"))
			return err
		}

		mgmtv3.ClusterConditionPrometheusOperatorDeployed.False(cluster)
		mgmtv3.ClusterConditionPrometheusOperatorDeployed.Reason(cluster, "")
		mgmtv3.ClusterConditionPrometheusOperatorDeployed.Message(cluster, "")
	}

	return nil
}

func allOwnedProjectsMonitoringDisabling(projectClient mgmtv3.ProjectInterface) (bool, error) {
	ownedProjectList, err := projectClient.List(metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	for _, ownedProject := range ownedProjectList.Items {
		if ownedProject.Spec.EnableProjectMonitoring {
			return false, nil
		}
	}

	return true, nil
}

func deploySystemMonitor(cluster *mgmtv3.Cluster, app *appHandler) (backErr error) {
	appName, appTargetNamespace := monitoring.SystemMonitoringInfo()

	appCatalogID := settings.SystemMonitoringCatalogID.Get()
	err := monitoring.DetectAppCatalogExistence(appCatalogID, app.cattleTemplateVersionClient)
	if err != nil {
		return errors.Wrapf(err, "failed to ensure catalog %q", appCatalogID)
	}

	appDeployProjectID, err := monitoring.GetSystemProjectID(app.cattleProjectClient)
	if err != nil {
		return errors.Wrap(err, "failed to get System Project ID")
	}

	appProjectName, err := monitoring.EnsureAppProjectName(app.agentNamespaceClient, appDeployProjectID, cluster.Name, appTargetNamespace)
	if err != nil {
		return errors.Wrap(err, "failed to ensure monitoring project name")
	}

	appAnswers := map[string]string{
		"enabled":      "true",
		"apiGroup":     monitoring.APIVersion.Group,
		"nameOverride": "prometheus-operator",
	}

	mustAppAnswers := map[string]string{
		"operator.apiGroup":     monitoring.APIVersion.Group,
		"operator.nameOverride": "prometheus-operator",
	}

	// take operator answers from overwrite answers
	for ansKey, ansVal := range monitoring.GetOverwroteAppAnswers(cluster.Annotations) {
		if strings.HasPrefix(ansKey, "operator.") {
			appAnswers[ansKey] = ansVal
		}
	}

	// cannot overwrite mustAppAnswers
	for mustKey, mustVal := range mustAppAnswers {
		appAnswers[mustKey] = mustVal
	}

	creator, err := app.systemAccountManager.GetSystemUser(cluster.Name)
	if err != nil {
		return err
	}

	annotations := map[string]string{
		"cluster.cattle.io/addon": appName,
		creatorIDAnno:             creator.Name,
	}

	targetApp := &projectv3.App{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
			Labels:      monitoring.OwnedLabels(appName, appTargetNamespace, appProjectName, monitoring.SystemLevel),
			Name:        appName,
			Namespace:   appDeployProjectID,
		},
		Spec: projectv3.AppSpec{
			Answers:         appAnswers,
			Description:     "Prometheus Operator for Rancher Monitoring",
			ExternalID:      appCatalogID,
			ProjectName:     appProjectName,
			TargetNamespace: appTargetNamespace,
		},
	}

	// redeploy operator App forcibly if cannot find the workload
	var forceRedeploy bool
	appWorkload, err := app.agentDeploymentClient.GetNamespaced(appTargetNamespace, fmt.Sprintf("prometheus-operator-%s", appName), metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to get deployment %s/prometheus-operator-%s", appTargetNamespace, appName)
	}
	if appWorkload == nil || appWorkload.Name == "" || appWorkload.DeletionTimestamp != nil {
		forceRedeploy = true
	}

	_, err = monitoring.DeployApp(app.cattleAppClient, appDeployProjectID, targetApp, forceRedeploy)
	if err != nil {
		return errors.Wrap(err, "failed to ensure prometheus operator app")
	}

	return nil
}
