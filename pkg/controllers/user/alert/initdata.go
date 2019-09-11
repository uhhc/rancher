package alert

import (
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/rancher/rancher/pkg/controllers/user/alert/common"
	"github.com/rancher/rancher/pkg/controllers/user/alert/manager"
	v1 "github.com/rancher/types/apis/core/v1"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	nodeAlertGroupName = "node-alert"
)

var (
	windowNodeLabel = labels.Set(map[string]string{"kubernetes.io/os": "windows"}).AsSelector()
)

func initClusterPreCanAlerts(clusterAlertGroups v3.ClusterAlertGroupInterface, clusterAlertRules v3.ClusterAlertRuleInterface, clusterName string) {
	name := "etcd-alert"
	group := &v3.ClusterAlertGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterGroupSpec{
			ClusterName: clusterName,
			CommonGroupField: v3.CommonGroupField{
				Description: "Alert for etcd leader existence, db size",
				DisplayName: "A set of alerts for etcd",
				TimingField: defaultTimingField,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertGroups.Create(group); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan alert group for etcd: %v", err)
	}

	name = "no-leader"
	rule := &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: clusterName,
			GroupName:   common.GetGroupID(clusterName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityCritical,
				DisplayName: "Etcd member has no leader",
				TimingField: defaultTimingField,
			},
			MetricRule: &v3.MetricRule{
				Description:    "Etcd member has no leader",
				Expression:     `etcd_server_has_leader`,
				Comparison:     manager.ComparisonNotEqual,
				Duration:       "3m",
				ThresholdValue: 1,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}

	name = "high-number-of-leader-changes"
	rule = &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: clusterName,
			GroupName:   common.GetGroupID(clusterName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityWarning,
				DisplayName: "A high number of leader changes within the etcd cluster are happening",
				TimingField: defaultTimingField,
			},
			MetricRule: &v3.MetricRule{
				Description:    "Etcd instance has seen high number of leader changes within the last hour",
				Expression:     `increase(etcd_server_leader_changes_seen_total[1h])`,
				Comparison:     manager.ComparisonGreaterThan,
				Duration:       "3m",
				ThresholdValue: 3,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}

	name = "db-over-size"
	rule = &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: clusterName,
			GroupName:   common.GetGroupID(clusterName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityWarning,
				DisplayName: "Database usage close to the quota 500M",
				TimingField: defaultTimingField,
			},
			MetricRule: &v3.MetricRule{
				Description:    "Shows the etcd database size including free space waiting for defragmentation close to the quota",
				Expression:     `sum(etcd_debugging_mvcc_db_total_size_in_bytes)`,
				Comparison:     manager.ComparisonGreaterThan,
				Duration:       "3m",
				ThresholdValue: 524288000,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}

	inherited := false
	name = "etcd-system-service"
	rule = &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: clusterName,
			GroupName:   common.GetGroupID(clusterName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityCritical,
				DisplayName: "Etcd is unavailable",
				Inherited:   &inherited,
				TimingField: v3.TimingField{
					GroupWaitSeconds:      600,
					GroupIntervalSeconds:  180,
					RepeatIntervalSeconds: 3600,
				},
			},
			SystemServiceRule: &v3.SystemServiceRule{
				Condition: "etcd",
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}

	name = "kube-components-alert"
	group = &v3.ClusterAlertGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterGroupSpec{
			ClusterName: clusterName,
			CommonGroupField: v3.CommonGroupField{
				Description: "Alert for kube components api server, scheduler, controller manager",
				DisplayName: "A set of alerts for kube components",
				TimingField: defaultTimingField,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertGroups.Create(group); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for controller manager, scheduler: %v", err)
	}

	name = "scheduler-system-service"
	rule = &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: clusterName,
			GroupName:   common.GetGroupID(clusterName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityCritical,
				DisplayName: "Scheduler is unavailable",
				TimingField: defaultTimingField,
			},
			SystemServiceRule: &v3.SystemServiceRule{
				Condition: "scheduler",
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}

	name = "controllermanager-system-service"
	rule = &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: clusterName,
			GroupName:   common.GetGroupID(clusterName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityCritical,
				DisplayName: "Controller Manager is unavailable",
				TimingField: defaultTimingField,
			},
			SystemServiceRule: &v3.SystemServiceRule{
				Condition: "controller-manager",
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}

	name = nodeAlertGroupName
	group = &v3.ClusterAlertGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterGroupSpec{
			ClusterName: clusterName,
			CommonGroupField: v3.CommonGroupField{
				Description: "Alert for Node Memory, CPU, Disk Usage",
				DisplayName: "A set of alerts for node",
				TimingField: defaultTimingField,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertGroups.Create(group); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for node: %v", err)
	}

	name = "node-disk-running-full"
	rule = &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: clusterName,
			GroupName:   common.GetGroupID(clusterName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityCritical,
				DisplayName: "Node disk is running full within 24 hours",
				TimingField: defaultTimingField,
			},
			MetricRule: &v3.MetricRule{
				Description: "Device on node is running full within the next 24 hours",
				Expression:  `predict_linear(node_filesystem_free_bytes{mountpoint!~"^/etc/(?:resolv.conf|hosts|hostname)$"}[6h], 3600 * 24) < 0`,
				Comparison:  manager.ComparisonHasValue,
				Duration:    "10m",
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}

	name = "high-memmory"
	rule = &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: clusterName,
			GroupName:   common.GetGroupID(clusterName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityWarning,
				DisplayName: "High node memory utilization",
				TimingField: defaultTimingField,
			},
			MetricRule: &v3.MetricRule{
				Description:    "Node memory utilization is over 80%",
				Expression:     `(1 - sum(node_memory_MemAvailable_bytes) by (instance) / sum(node_memory_MemTotal_bytes) by (instance)) * 100`,
				Comparison:     manager.ComparisonGreaterOrEqual,
				Duration:       "3m",
				ThresholdValue: 80,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}

	name = "high-cpu-load"
	rule = &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: clusterName,
			GroupName:   common.GetGroupID(clusterName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityWarning,
				DisplayName: "High cpu load",
				TimingField: defaultTimingField,
			},
			MetricRule: &v3.MetricRule{
				Description:    "The cpu load is higher than 100",
				Expression:     `sum(node_load1) by (node)  / sum(machine_cpu_cores) by (node) * 100`,
				Comparison:     manager.ComparisonGreaterThan,
				Duration:       "3m",
				ThresholdValue: 100,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}

	name = "event-alert"
	group = &v3.ClusterAlertGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterGroupSpec{
			ClusterName: clusterName,
			CommonGroupField: v3.CommonGroupField{
				Description: "Alert for receiving resource event",
				DisplayName: "A set of alerts when event happened",
				TimingField: defaultTimingField,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertGroups.Create(group); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for event: %v", err)
	}

	name = "deployment-event-alert"
	rule = &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: clusterName,
			GroupName:   common.GetGroupID(clusterName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityWarning,
				DisplayName: "Get warning deployment event",
				TimingField: defaultTimingField,
			},
			EventRule: &v3.EventRule{
				EventType:    "Warning",
				ResourceKind: "Deployment",
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}
}

type ProjectLifecycle struct {
	projectAlertGroups v3.ProjectAlertGroupInterface
	projectAlertRules  v3.ProjectAlertRuleInterface
	clusterName        string
}

//Create built-in project alerts
func (l *ProjectLifecycle) Create(obj *v3.Project) (runtime.Object, error) {
	name := "projectalert-workload-alert"
	projectName := obj.Name
	projectID := fmt.Sprintf("%s:%s", obj.Namespace, obj.Name)
	group := &v3.ProjectAlertGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: projectName,
		},
		Spec: v3.ProjectGroupSpec{
			ProjectName: projectID,
			CommonGroupField: v3.CommonGroupField{
				DisplayName: "A set of alerts for workload, pod, container",
				Description: "Alert for cpu, memory, disk, network",
				TimingField: defaultTimingField,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := l.projectAlertGroups.Create(group); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create built-in rules for deployment: %v", err)
	}

	name = "less-than-half-workload-available"
	rule := &v3.ProjectAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: projectName,
		},
		Spec: v3.ProjectAlertRuleSpec{
			ProjectName: projectID,
			GroupName:   common.GetGroupID(projectName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityCritical,
				DisplayName: "Less than half workload available",
				TimingField: defaultTimingField,
			},
			WorkloadRule: &v3.WorkloadRule{
				Selector: map[string]string{
					"app": "workload",
				},
				AvailablePercentage: 50,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := l.projectAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}

	name = "memory-close-to-resource-limited"
	rule = &v3.ProjectAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: projectName,
		},
		Spec: v3.ProjectAlertRuleSpec{
			ProjectName: projectID,
			GroupName:   common.GetGroupID(projectName, group.Name),
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityWarning,
				DisplayName: "Memory usage close to the quota",
				TimingField: defaultTimingField,
			},
			MetricRule: &v3.MetricRule{
				Description:    "Container using memory close to the quota",
				Expression:     `sum(container_memory_working_set_bytes) by (pod_name, container_name) / sum(label_join(label_join(kube_pod_container_resource_limits_memory_bytes,"pod_name", "", "pod"),"container_name", "", "container")) by (pod_name, container_name)`,
				Comparison:     manager.ComparisonGreaterThan,
				Duration:       "3m",
				ThresholdValue: 1,
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := l.projectAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		logrus.Warnf("Failed to create precan rules for %s: %v", name, err)
	}
	return obj, nil
}

func (l *ProjectLifecycle) Updated(obj *v3.Project) (runtime.Object, error) {
	return obj, nil
}

func (l *ProjectLifecycle) Remove(obj *v3.Project) (runtime.Object, error) {
	return obj, nil
}

func getCommonRuleField(groupID, displayName, severity string) v3.CommonRuleField {
	return v3.CommonRuleField{
		DisplayName: displayName,
		Severity:    severity,
		TimingField: defaultTimingField,
	}
}

type windowsNodeSyner struct {
	clusterName       string
	clusterAlertRules v3.ClusterAlertRuleInterface
	nodeLister        v1.NodeLister
}

func (l *windowsNodeSyner) Sync(key string, obj *corev1.Node) (runtime.Object, error) {
	windowsNodes, err := l.nodeLister.List(metav1.NamespaceAll, windowNodeLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list nodes")
	}

	if len(windowsNodes) != 0 {
		return obj, l.initClusterWindowsAlert()
	}

	return obj, nil
}

func (l *windowsNodeSyner) initClusterWindowsAlert() error {
	groupName := common.GetGroupID(l.clusterName, nodeAlertGroupName)
	name := "windows-node-disk-running-full"

	exsitWindowsAlert, err := l.clusterAlertRules.Controller().Lister().Get(l.clusterName, name)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "Failed to get alert rules %s", name)
	}

	if exsitWindowsAlert != nil {
		return nil
	}

	rule := &v3.ClusterAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v3.ClusterAlertRuleSpec{
			ClusterName: l.clusterName,
			GroupName:   groupName,
			CommonRuleField: v3.CommonRuleField{
				Severity:    SeverityCritical,
				DisplayName: "Windows node disk is running full within 24 hours",
				TimingField: defaultTimingField,
			},
			MetricRule: &v3.MetricRule{
				Description: "Device on node is running full within the next 24 hours",
				Expression:  `predict_linear(node_filesystem_free_bytes{job=~"expose-node-metrics-windows", device!~"HarddiskVolume.+"}[6h], 3600 * 24) < 0`,
				Comparison:  manager.ComparisonHasValue,
				Duration:    "10m",
			},
		},
		Status: v3.AlertStatus{
			AlertState: "active",
		},
	}

	if _, err := l.clusterAlertRules.Create(rule); err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "Failed to create precan rules %s", name)
	}
	return nil
}
