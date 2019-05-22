package utils

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/rancher/rancher/pkg/controllers/user/nslabels"
	"github.com/rancher/rancher/pkg/project"
	"github.com/rancher/types/apis/core/v1"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	projv3 "github.com/rancher/types/apis/project.cattle.io/v3"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

func EnsureAppProjectName(userNSClient v1.NamespaceInterface, ownedProjectID, clusterName, appTargetNamespace string) (string, error) {
	// detect Namespace
	deployNamespace, err := userNSClient.Get(appTargetNamespace, metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return "", errors.Wrapf(err, "failed to find %q Namespace", appTargetNamespace)
	}
	deployNamespace = deployNamespace.DeepCopy()

	if deployNamespace.Name == appTargetNamespace {
		if deployNamespace.DeletionTimestamp != nil {
			return "", fmt.Errorf("stale %q Namespace is still on terminating", appTargetNamespace)
		}
	} else {
		deployNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: appTargetNamespace,
			},
		}

		if deployNamespace, err = userNSClient.Create(deployNamespace); err != nil && !kerrors.IsAlreadyExists(err) {
			return "", errors.Wrapf(err, "failed to create %q Namespace", appTargetNamespace)
		}
	}

	// move Namespace into a project
	expectedAppProjectName := fmt.Sprintf("%s:%s", clusterName, ownedProjectID)
	appProjectName := ""
	if projectName, ok := deployNamespace.Annotations[nslabels.ProjectIDFieldLabel]; ok {
		appProjectName = projectName
	}
	if appProjectName != expectedAppProjectName {
		appProjectName = expectedAppProjectName
		if deployNamespace.Annotations == nil {
			deployNamespace.Annotations = make(map[string]string, 2)
		}

		deployNamespace.Annotations[nslabels.ProjectIDFieldLabel] = appProjectName

		_, err := userNSClient.Update(deployNamespace)
		if err != nil {
			return "", errors.Wrapf(err, "failed to move Namespace %s into a Project", appTargetNamespace)
		}
	}

	return appProjectName, nil
}

func GetSystemProjectID(cattleProjectsClient v3.ProjectInterface) (string, error) {
	// fetch all system Projects
	cattletSystemProjects, _ := cattleProjectsClient.List(metav1.ListOptions{
		LabelSelector: "authz.management.cattle.io/system-project=true",
	})

	var systemProject *v3.Project
	cattletSystemProjects = cattletSystemProjects.DeepCopy()
	for _, defaultProject := range cattletSystemProjects.Items {
		systemProject = &defaultProject

		if defaultProject.Spec.DisplayName == project.System {
			break
		}
	}
	if systemProject == nil {
		return "", fmt.Errorf("failed to find any cattle system project")
	}

	return systemProject.Name, nil
}

func DeployApp(mgmtAppClient projv3.AppInterface, projectID string, createOrUpdateApp *projv3.App, forceRedeploy bool) (*projv3.App, error) {
	if createOrUpdateApp == nil {
		return nil, errors.New("cannot deploy a nil App")
	}
	var rtn *projv3.App

	appName := createOrUpdateApp.Name
	app, err := mgmtAppClient.GetNamespaced(projectID, appName, metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, errors.Wrapf(err, "failed to query %q App in %s Project", appName, projectID)
	}

	if app.DeletionTimestamp != nil {
		return nil, fmt.Errorf("stale %q App in %s Project is still on terminating", appName, projectID)
	}

	if app.Name == "" {
		logrus.Infof("Create app %s/%s", app.Spec.TargetNamespace, app.Name)
		if rtn, err = mgmtAppClient.Create(createOrUpdateApp); err != nil {
			return nil, errors.Wrapf(err, "failed to create %q App", appName)
		}
	} else {
		app = app.DeepCopy()
		app.Spec.Answers = createOrUpdateApp.Spec.Answers

		// clean up status
		if forceRedeploy {
			if app.Spec.Answers == nil {
				app.Spec.Answers = make(map[string]string, 1)
			}
			app.Spec.Answers["redeployTs"] = fmt.Sprintf("%d", time.Now().Unix())
		}

		if rtn, err = mgmtAppClient.Update(app); err != nil {
			return nil, errors.Wrapf(err, "failed to update %q App", appName)
		}
	}

	return rtn, nil
}
