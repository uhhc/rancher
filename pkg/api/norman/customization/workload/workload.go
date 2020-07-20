package workload

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/rancher/norman/api/access"
	"github.com/rancher/norman/api/handler"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/norman/types/values"
	projectclient "github.com/rancher/rancher/pkg/client/generated/project/v3"
	"github.com/rancher/rancher/pkg/clustermanager"
	appsv1 "github.com/rancher/rancher/pkg/generated/norman/apps/v1"
	projectschema "github.com/rancher/rancher/pkg/schemas/project.cattle.io/v3"
	schema "github.com/rancher/rancher/pkg/schemas/project.cattle.io/v3"
	"github.com/rancher/rancher/pkg/types/config"
	"github.com/sirupsen/logrus"
	k8sappsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	workloadRevisions    = "revisions"
	DeprecatedRollbackTo = "deprecated.deployment.rollback.to"
)

var (
	allowRedeployTypes = map[string]bool{"cronJob": true, "deployment": true, "replicationController": true, "statefulSet": true, "daemonSet": true, "replicaSet": true}
)

type ActionWrapper struct {
	ClusterManager *clustermanager.Manager
}

func (a ActionWrapper) ActionHandler(actionName string, action *types.Action, apiContext *types.APIContext) error {
	var deployment projectclient.Workload
	accessError := access.ByID(apiContext, &projectschema.Version, "workload", apiContext.ID, &deployment)
	if accessError != nil {
		return httperror.NewAPIError(httperror.InvalidReference, "Error accessing workload")
	}
	namespace, name := splitID(deployment.ID)
	switch actionName {
	case "rollback":
		clusterName := a.ClusterManager.ClusterName(apiContext)
		if clusterName == "" {
			return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Cluster name empty %s", deployment.ID))
		}
		clusterContext, err := a.ClusterManager.UserContext(clusterName)
		if err != nil {
			return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Error getting cluster context %s", deployment.ID))
		}
		return a.rollbackDeployment(apiContext, clusterContext, actionName, deployment, namespace, name)

	case "pause":
		if deployment.Paused {
			return httperror.NewAPIError(httperror.InvalidAction, fmt.Sprintf("Deployment %s already paused", deployment.ID))
		}
		return updatePause(apiContext, true, deployment, "pause")

	case "resume":
		if !deployment.Paused {
			return httperror.NewAPIError(httperror.InvalidAction, fmt.Sprintf("Pause deployment %s before resume", deployment.ID))
		}
		return updatePause(apiContext, false, deployment, "resume")
	case "redeploy":
		return updateTimestamp(apiContext, deployment)
	}
	return nil
}

func fetchRevisionFor(apiContext *types.APIContext, rollbackInput *projectclient.DeploymentRollbackInput, namespace string, name string, currRevision string) string {
	rollbackTo := rollbackInput.ReplicaSetID
	if rollbackTo == "" {
		revisionNum, _ := convert.ToNumber(currRevision)
		return convert.ToString(revisionNum - 1)
	}
	data := getRevisions(apiContext, namespace, name, rollbackTo)
	if len(data) > 0 {
		return convert.ToString(values.GetValueN(data[0], "workloadAnnotations", "deployment.kubernetes.io/revision"))
	}
	return ""
}

func getRevisions(apiContext *types.APIContext, namespace string, name string, requestedID string) []map[string]interface{} {
	var data, replicaSets []map[string]interface{}
	options := map[string]string{"hidden": "true"}
	conditions := []*types.QueryCondition{
		types.NewConditionFromString("namespaceId", types.ModifierEQ, []string{namespace}...),
	}
	if requestedID != "" {
		// want a specific replicaSet
		conditions = append(conditions, types.NewConditionFromString("id", types.ModifierEQ, []string{requestedID}...))
	}

	if err := access.List(apiContext, &projectschema.Version, projectclient.ReplicaSetType, &types.QueryOptions{Options: options, Conditions: conditions}, &replicaSets); err == nil {
		for _, replicaSet := range replicaSets {
			ownerReferences := convert.ToMapSlice(replicaSet["ownerReferences"])
			for _, ownerReference := range ownerReferences {
				kind := convert.ToString(ownerReference["kind"])
				ownerName := convert.ToString(ownerReference["name"])
				if kind == "Deployment" && name == ownerName {
					data = append(data, replicaSet)
					continue
				}
			}
		}
	}
	return data
}

func updatePause(apiContext *types.APIContext, value bool, deployment projectclient.Workload, actionName string) error {
	data, err := convert.EncodeToMap(deployment)
	if err == nil {
		values.PutValue(data, value, "paused")
		err = update(apiContext, data, deployment.ID)
	}
	if err != nil {
		return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Error updating workload %s by %s : %s", deployment.ID, actionName, err.Error()))
	}
	return nil
}

func update(apiContext *types.APIContext, data map[string]interface{}, ID string) error {
	workloadSchema := apiContext.Schemas.Schema(&schema.Version, "workload")
	_, err := workloadSchema.Store.Update(apiContext, workloadSchema, data, ID)
	return err
}

func (a ActionWrapper) rollbackDeployment(apiContext *types.APIContext, clusterContext *config.UserContext,
	actionName string, deployment projectclient.Workload, namespace string, name string) error {
	input, err := handler.ParseAndValidateActionBody(apiContext, apiContext.Schemas.Schema(&projectschema.Version,
		projectclient.DeploymentRollbackInputType))
	if err != nil {
		return httperror.NewAPIError(httperror.InvalidBodyContent,
			fmt.Sprintf("Failed to parse action body: %v", err))
	}
	rollbackInput := &projectclient.DeploymentRollbackInput{}
	if err := mapstructure.Decode(input, rollbackInput); err != nil {
		return httperror.NewAPIError(httperror.InvalidBodyContent,
			fmt.Sprintf("Failed to parse body: %v", err))
	}
	currRevision := deployment.WorkloadAnnotations["deployment.kubernetes.io/revision"]
	if currRevision == "1" {
		httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("No revision for rolling back %s", deployment.ID))
	}
	// if deployment's apiversion is apps/v1, we update the object, so getting it from etcd instead of cache
	depl, err := clusterContext.Apps.Deployments(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Error getting deployment %v: %v", name, err))
	}
	deploymentVersion, err := k8sschema.ParseGroupVersion(depl.APIVersion)
	if err != nil {
		return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Error parsing api version for deployment %v: %v", name, err))
	}
	if deploymentVersion == k8sappsv1.SchemeGroupVersion {
		logrus.Debugf("Deployment apiversion is apps/v1")
		// DeploymentRollback & RollbackTo are deprecated in apps/v1
		// only way to rollback is update deployment podSpec with replicaSet podSpec
		split := strings.SplitN(rollbackInput.ReplicaSetID, ":", 3)
		if len(split) != 3 || split[0] != appsv1.ReplicaSetResource.SingularName {
			return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Invalid ReplicaSet %s", rollbackInput.ReplicaSetID))
		}
		replicaNamespace, replicaName := split[1], split[2]
		rs, err := clusterContext.Apps.ReplicaSets("").Controller().Lister().Get(replicaNamespace, replicaName)
		if err != nil {
			logrus.Debugf("ReplicaSet not found in cache, fetching from etcd")
			rs, err = clusterContext.Apps.ReplicaSets("").GetNamespaced(replicaNamespace, replicaName, v1.GetOptions{})
			if err != nil {
				return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("ReplicaSet %s not found for deployment %s", rollbackInput.ReplicaSetID, deployment.ID))
			}
		}
		toUpdateDepl := depl.DeepCopy()
		toUpdateDepl.Spec.Template.Spec = rs.Spec.Template.Spec
		_, err = clusterContext.Apps.Deployments("").Update(toUpdateDepl)
		if err != nil {
			return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Error updating workload under apps/v1 %s by %s : %s", deployment.ID, actionName, err.Error()))
		}
		return nil
	}

	revision := fetchRevisionFor(apiContext, rollbackInput, namespace, name, currRevision)
	logrus.Debugf("rollbackInput %v", revision)
	if revision == "" {
		return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("ReplicaSet %s doesn't exist for deployment %s", rollbackInput.ReplicaSetID, deployment.ID))
	}
	revisionNum, err := convert.ToNumber(revision)
	if err != nil {
		return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Error getting revision number %s for %s : %s", revision, deployment.ID, err.Error()))
	}
	data := map[string]interface{}{}
	data["kind"] = "DeploymentRollback"
	data["apiVersion"] = "extensions/v1beta1"
	data["name"] = name
	data["rollbackTo"] = map[string]interface{}{"revision": revisionNum}
	deploymentRollback, err := json.Marshal(data)
	if err != nil {
		return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Error getting DeploymentRollback for %s %s", rollbackInput.ReplicaSetID, err.Error()))
	}
	err = clusterContext.UnversionedClient.Post().Prefix("apis/extensions/v1beta1/").Namespace(namespace).
		Resource("deployments").Name(name).SubResource("rollback").Body(deploymentRollback).Do(apiContext.Request.Context()).Error()
	if err != nil {
		return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Error updating workload %s by %s : %s", deployment.ID, actionName, err.Error()))
	}
	return nil
}
func updateTimestamp(apiContext *types.APIContext, workload projectclient.Workload) error {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	data, err := convert.EncodeToMap(workload)
	if err != nil {
		return httperror.WrapAPIError(err, httperror.ServerError, "Failed to parse workload")
	}
	values.PutValue(data, timestamp, "annotations", "cattle.io/timestamp")
	err = update(apiContext, data, workload.ID)
	if err != nil {
		return httperror.NewAPIError(httperror.ServerError, fmt.Sprintf("Error redeploying workload %s : %s", workload.ID, err.Error()))
	}
	return nil
}
func (h Handler) LinkHandler(apiContext *types.APIContext, next types.RequestHandler) error {
	if apiContext.Link == workloadRevisions {
		var deployment projectclient.Workload
		if err := access.ByID(apiContext, &projectschema.Version, "workload", apiContext.ID, &deployment); err == nil {
			namespace, deploymentName := splitID(deployment.ID)
			data := getRevisions(apiContext, namespace, deploymentName, "")
			apiContext.Type = projectclient.ReplicaSetType
			apiContext.WriteResponse(http.StatusOK, data)
		}
		return nil
	}
	return httperror.NewAPIError(httperror.NotFound, "Link not found")
}

func Formatter(apiContext *types.APIContext, resource *types.RawResource) {
	workloadID := resource.ID
	workloadSchema := apiContext.Schemas.Schema(&schema.Version, "workload")
	resource.Links["self"] = apiContext.URLBuilder.ResourceLinkByID(workloadSchema, workloadID)
	resource.Links["remove"] = apiContext.URLBuilder.ResourceLinkByID(workloadSchema, workloadID)
	resource.Links["update"] = apiContext.URLBuilder.ResourceLinkByID(workloadSchema, workloadID)
	//add redeploy action to the workload types that support redeploy
	if _, ok := allowRedeployTypes[resource.Type]; ok {
		resource.Actions["redeploy"] = apiContext.URLBuilder.ActionLinkByID(workloadSchema, workloadID, "redeploy")
	}

	delete(resource.Values, "nodeId")
}

func DeploymentFormatter(apiContext *types.APIContext, resource *types.RawResource) {
	workloadID := resource.ID
	workloadSchema := apiContext.Schemas.Schema(&schema.Version, "workload")
	Formatter(apiContext, resource)
	resource.Links["revisions"] = apiContext.URLBuilder.ResourceLinkByID(workloadSchema, workloadID) + "/" + workloadRevisions
	resource.Actions["pause"] = apiContext.URLBuilder.ActionLinkByID(workloadSchema, workloadID, "pause")
	resource.Actions["resume"] = apiContext.URLBuilder.ActionLinkByID(workloadSchema, workloadID, "resume")
	resource.Actions["rollback"] = apiContext.URLBuilder.ActionLinkByID(workloadSchema, workloadID, "rollback")
}

type Handler struct {
}

func splitID(id string) (string, string) {
	namespace := ""
	parts := strings.SplitN(id, ":", 3)
	if len(parts) == 3 {
		namespace = parts[1]
		id = parts[2]
	}

	return namespace, id
}
