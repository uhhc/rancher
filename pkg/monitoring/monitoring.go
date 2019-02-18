package monitoring

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/rancher/norman/types"
	"github.com/rancher/rancher/pkg/ref"
	mgmtv3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AppLevel string

const (
	SystemLevel  AppLevel = "system"
	ClusterLevel AppLevel = "cluster"
	ProjectLevel AppLevel = "project"
)

const (
	cattleNamespaceName                    = "cattle-prometheus"
	cattleCreatorIDAnnotationKey           = "field.cattle.io/creatorId"
	cattleOverwriteAppAnswersAnnotationKey = "field.cattle.io/overwriteAppAnswers"
)

const (
	//CattleMonitoringLabelKey The label info of Namespace
	cattleMonitoringLabelKey = "monitoring.coreos.com"

	// The label info of App, RoleBinding
	appNameLabelKey            = cattleMonitoringLabelKey + "/appName"
	appTargetNamespaceLabelKey = cattleMonitoringLabelKey + "/appTargetNamespace"
	appProjectIDLabelKey       = cattleMonitoringLabelKey + "/projectID"
	appClusterIDLabelKey       = cattleMonitoringLabelKey + "/clusterID"
	appLevelLabelKey           = cattleMonitoringLabelKey + "/level"

	// The names of App
	systemLevelAppName              = "monitoring-operator"
	clusterLevelAppName             = "cluster-monitoring"
	projectLevelAppName             = "project-monitoring"
	clusterLevelAlertManagerAppName = "cluster-alerting"

	// The headless service name of Prometheus
	alertManagerHeadlessServiceName = "alertmanager-operated"
	prometheusHeadlessServiceName   = "prometheus-operated"

	//CattlePrometheusRuleLabelKey The label info of PrometheusRule
	CattlePrometheusRuleLabelKey             = "source"
	CattleAlertingPrometheusRuleLabelValue   = "rancher-alert"
	CattleMonitoringPrometheusRuleLabelValue = "rancher-monitoring"
)

var (
	APIVersion = types.APIVersion{
		Version: "v1",
		Group:   "monitoring.coreos.com",
		Path:    "/v3/project",
	}

	tplRegexp = &templateRegexp{
		r: regexp.MustCompile(`(?P<middlePrefix>.+)#\((?P<roots>.+)\)`),
	}
)

func OwnedAppListOptions(clusterID, appName, appTargetNamespace string) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s, %s=%s, %s=%s", appClusterIDLabelKey, clusterID, appNameLabelKey, appName, appTargetNamespaceLabelKey, appTargetNamespace),
	}
}

func CopyCreatorID(toAnnotations, fromAnnotations map[string]string) map[string]string {
	if val, exist := fromAnnotations[cattleCreatorIDAnnotationKey]; exist {
		if toAnnotations == nil {
			toAnnotations = make(map[string]string, 2)
		}

		toAnnotations[cattleCreatorIDAnnotationKey] = val
	}

	return toAnnotations
}

func AppendAppOverwritingAnswers(toAnnotations map[string]string, appOverwriteAnswers string) map[string]string {
	if len(strings.TrimSpace(appOverwriteAnswers)) != 0 {
		if toAnnotations == nil {
			toAnnotations = make(map[string]string, 2)
		}

		toAnnotations[cattleOverwriteAppAnswersAnnotationKey] = appOverwriteAnswers
	}

	return toAnnotations
}

func OwnedLabels(appName, appTargetNamespace, appProjectName string, level AppLevel) map[string]string {
	clusterID, projectID := ref.Parse(appProjectName)

	return map[string]string{
		appNameLabelKey:            appName,
		appTargetNamespaceLabelKey: appTargetNamespace,
		appProjectIDLabelKey:       projectID,
		appClusterIDLabelKey:       clusterID,
		appLevelLabelKey:           string(level),
	}
}

func SystemMonitoringInfo() (appName, appTargetNamespace string) {
	return systemLevelAppName, cattleNamespaceName
}

func ClusterMonitoringInfo() (appName, appTargetNamespace string) {
	return clusterLevelAppName, cattleNamespaceName
}

func ClusterAlertManagerInfo() (appName, appTargetNamespace string) {
	return clusterLevelAlertManagerAppName, cattleNamespaceName
}

func ProjectMonitoringInfo(projectName string) (appName, appTargetNamespace string) {
	return projectLevelAppName, fmt.Sprintf("%s-%s", cattleNamespaceName, projectName)
}

func ClusterAlertManagerEndpoint() (headlessServiceName, namespace, port string) {
	return alertManagerHeadlessServiceName, cattleNamespaceName, "9093"
}

func ClusterPrometheusEndpoint() (headlessServiceName, namespace, port string) {
	return prometheusHeadlessServiceName, cattleNamespaceName, "9090"
}

func ProjectPrometheusEndpoint(projectName string) (headlessServiceName, namespace, port string) {
	return prometheusHeadlessServiceName, fmt.Sprintf("%s-%s", cattleNamespaceName, projectName), "9090"
}

/*OverwriteAppAnswers Usage
## special key prefix
_tpl- [priority low] ->  regex ${value} = ${middle-prefix}#(${root1,root2,...}), then generate ${root*}.${middle-prefix} as prefix-key

## example

### input
				key 				 	|           			value
-----------------------------------------------------------------------------------------------
_tpl-Node_Selector       	     		| nodeSelector#(prometheus,grafana,exporter-kube-state)
_tpl-Storage_Class       	     		| persistence#(prometheus,grafana)
-----------------------------------------------------------------------------------------------
prometheus.retention				 	| 360h
exporter-node.ports.metrics.port	 	| 9100
grafana.persistence.enabled             | false
nodeSelector.region		 				| region-a
nodeSelector.zone         				| zone-b
persistence.enabled       				| true
persistence.storageClass  				| default
persistence.accessMode    				| ReadWriteOnce
persistence.size          				| 50Gi

### output
				key 				 	|           			value
-----------------------------------------------------------------------------------------------
prometheus.retention				 	| 360h
exporter-node.ports.metrics.port	 	| 9100
prometheus.nodeSelector.region		 	| region-a
prometheus.nodeSelector.zone         	| zone-b
grafana.nodeSelector.region		 		| region-a
grafana.nodeSelector.zone         		| zone-b
exporter-kube-state.nodeSelector.region	| region-a
exporter-kube-state.nodeSelector.zone   | zone-b
prometheus.persistence.enabled       	| true
prometheus.persistence.storageClass  	| default
prometheus.persistence.accessMode    	| ReadWriteOnce
prometheus.persistence.size          	| 50Gi
grafana.persistence.enabled       	 	| false         // can't overwrite by low priority
grafana.persistence.storageClass     	| default
grafana.persistence.accessMode       	| ReadWriteOnce
grafana.persistence.size             	| 50Gi

*/
func OverwriteAppAnswers(rawAnswers map[string]string, annotations map[string]string) map[string]string {
	overwriteAnswers := GetOverwroteAppAnswers(annotations)

	for specialKey, value := range overwriteAnswers {
		if strings.HasPrefix(specialKey, "_tpl-") {
			trr := tplRegexp.translate(value)
			for suffixKey, value := range overwriteAnswers {
				if strings.HasPrefix(suffixKey, trr.middlePrefix) {
					for _, prefixKey := range trr.roots {
						actualKey := fmt.Sprintf("%s.%s", prefixKey, suffixKey)

						rawAnswers[actualKey] = value
					}

					delete(overwriteAnswers, suffixKey)
				}
			}

			delete(overwriteAnswers, specialKey)
		}
	}

	for key, value := range overwriteAnswers {
		rawAnswers[key] = value
	}

	return rawAnswers
}

type templateRegexpResult struct {
	middlePrefix string
	roots        []string
}

type templateRegexp struct {
	r *regexp.Regexp
}

func (tr *templateRegexp) translate(value string) *templateRegexpResult {
	captures := &templateRegexpResult{}

	match := tr.r.FindStringSubmatch(value)
	if match == nil {
		return captures
	}

	for i, name := range tr.r.SubexpNames() {
		if name == "middlePrefix" {
			captures.middlePrefix = match[i]
		} else if name == "roots" {
			roots := strings.Split(match[i], ",")
			for _, root := range roots {
				root = strings.TrimSpace(root)
				if len(root) != 0 {
					captures.roots = append(captures.roots, root)
				}
			}
		}

	}

	return captures
}

func GetOverwroteAppAnswers(annotations map[string]string) map[string]string {
	overwritingAppAnswers := annotations[cattleOverwriteAppAnswersAnnotationKey]
	if len(overwritingAppAnswers) != 0 {
		var appOverwriteInput mgmtv3.MonitoringInput
		err := json.Unmarshal([]byte(overwritingAppAnswers), &appOverwriteInput)
		if err == nil {
			return appOverwriteInput.Answers
		}

		logrus.Errorf("failed to parse app overwrite input from %q, %v", overwritingAppAnswers, err)
	}

	return map[string]string{}
}
