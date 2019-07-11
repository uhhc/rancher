package systemimage

import (
	"encoding/json"
	"fmt"

	alerting "github.com/rancher/rancher/pkg/controllers/user/alert/deployer"
	logging "github.com/rancher/rancher/pkg/controllers/user/logging/deployer"
	pipeline "github.com/rancher/rancher/pkg/controllers/user/pipeline/upgrade"
	"github.com/rancher/rancher/pkg/project"
	"github.com/rancher/types/apis/apps/v1beta2"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

var systemProjectLabels = labels.Set(map[string]string{"authz.management.cattle.io/system-project": "true"})
var systemLibraryName = "system-library"

type Syncer struct {
	clusterName      string
	daemonsets       v1beta2.DaemonSetInterface
	daemonsetLister  v1beta2.DaemonSetLister
	deployments      v1beta2.DeploymentInterface
	deploymentLister v1beta2.DeploymentLister
	projectLister    v3.ProjectLister
	projects         v3.ProjectInterface
	userContext      *config.UserContext
	systemSercices   map[string]SystemService
}

func (s *Syncer) SyncProject(key string, obj *v3.Project) (runtime.Object, error) {
	if obj == nil || obj.DeletionTimestamp != nil {
		return nil, nil
	}
	return obj, s.Sync()
}

func (s *Syncer) SyncCatalog(key string, obj *v3.Catalog) (runtime.Object, error) {
	if obj == nil || obj.DeletionTimestamp != nil {
		return nil, nil
	}

	if obj.Name != systemLibraryName {
		return obj, nil
	}

	return obj, s.Sync()
}

func (s *Syncer) Sync() error {
	projects, err := s.projectLister.List(s.clusterName, systemProjectLabels.AsSelector())
	if err != nil {
		return fmt.Errorf("get projects failed when try to upgrade system tools, %v", err)
	}

	var systemProject *v3.Project
	for _, v := range projects {
		if v.Spec.DisplayName == project.System {
			systemProject = v.DeepCopy()
		}
	}

	if systemProject == nil {
		return nil
	}

	versionMap := make(map[string]string)
	curSysImageVersion := systemProject.Annotations[project.SystemImageVersionAnn]
	if curSysImageVersion != "" {
		if err = json.Unmarshal([]byte(curSysImageVersion), &versionMap); err != nil {
			return fmt.Errorf("unmashal current system service version failed, %v", err)
		}
	}

	changed := false
	for k, v := range s.systemSercices {
		oldVersion := versionMap[k]
		newVersion, err := v.Upgrade(oldVersion)
		if err != nil {
			return errors.Wrapf(err, "upgrade cluster %s system service %s failed", s.clusterName, k)
		}
		if oldVersion != newVersion {
			changed = true
			versionMap[k] = newVersion
		}
	}

	if !changed {
		return nil
	}

	newVersion, err := json.Marshal(versionMap)
	if err != nil {
		return fmt.Errorf("marshal new system service version %v failed, %v", versionMap, err)
	}

	systemProject.Annotations[project.SystemImageVersionAnn] = string(newVersion)
	_, err = s.projects.Update(systemProject)
	return err
}

func GetSystemImageVersion() (string, error) {
	versionMap := make(map[string]string)
	systemServices := getSystemService()
	for k, v := range systemServices {
		version, err := v.Version()
		if err != nil {
			return "", err
		}
		versionMap[k] = version
	}

	b, err := json.Marshal(versionMap)
	if err != nil {
		return "", fmt.Errorf("marshal toolsSystemImages failed: %v", err)
	}

	return string(b), nil
}

func getSystemService() map[string]SystemService {
	return map[string]SystemService{
		alerting.ServiceName: alerting.NewService(),
		logging.ServiceName:  logging.NewService(),
		pipeline.ServiceName: pipeline.NewService(),
	}
}
