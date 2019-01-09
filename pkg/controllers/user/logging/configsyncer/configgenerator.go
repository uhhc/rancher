package configsyncer

import (
	"sort"
	"strings"

	"github.com/rancher/norman/controller"
	"github.com/rancher/rancher/pkg/controllers/user/logging/generator"
	"github.com/rancher/rancher/pkg/project"
	"github.com/rancher/types/apis/core/v1"
	mgmtv3 "github.com/rancher/types/apis/management.cattle.io/v3"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func NewConfigGenerator(clusterName string, clusterLoggingLister mgmtv3.ClusterLoggingLister, projectLoggingLister mgmtv3.ProjectLoggingLister, namespaceLister v1.NamespaceLister) *ConfigGenerator {
	return &ConfigGenerator{
		clusterName:          clusterName,
		clusterLoggingLister: clusterLoggingLister,
		projectLoggingLister: projectLoggingLister,
		namespaceLister:      namespaceLister,
	}
}

type ConfigGenerator struct {
	clusterName          string
	clusterLoggingLister mgmtv3.ClusterLoggingLister
	projectLoggingLister mgmtv3.ProjectLoggingLister
	namespaceLister      v1.NamespaceLister
}

func (s *ConfigGenerator) GenerateClusterLoggingConfig(clusterLogging *mgmtv3.ClusterLogging, systemProjectID string) ([]byte, error) {
	if clusterLogging == nil {
		clusterLoggings, err := s.clusterLoggingLister.List("", labels.NewSelector())
		if err != nil {
			return nil, errors.Wrapf(err, "List cluster loggings failed")
		}
		if len(clusterLoggings) == 0 {
			return []byte{}, nil
		}

		clusterLogging = clusterLoggings[0]
	}

	var excludeNamespaces string
	if clusterLogging.Spec.ExcludeSystemComponent {
		var err error
		if excludeNamespaces, err = s.addExcludeNamespaces(systemProjectID); err != nil {
			return nil, err
		}
	}

	buf, err := generator.GenerateClusterConfig(clusterLogging.Spec, excludeNamespaces)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func (s *ConfigGenerator) GenerateProjectLoggingConfig(projectLoggings []*mgmtv3.ProjectLogging, systemProjectID string) ([]byte, error) {
	if len(projectLoggings) == 0 {
		allProjectLoggings, err := s.projectLoggingLister.List("", labels.NewSelector())
		if err != nil {
			return nil, errors.Wrapf(err, "List project loggings failed")
		}

		for _, logging := range allProjectLoggings {
			if controller.ObjectInCluster(s.clusterName, logging) {
				projectLoggings = append(projectLoggings, logging)
			}
		}

		if len(projectLoggings) == 0 {
			return []byte{}, nil
		}
	}

	sort.Slice(projectLoggings, func(i, j int) bool {
		return projectLoggings[i].Name < projectLoggings[j].Name
	})

	namespaces, err := s.namespaceLister.List(metav1.NamespaceAll, labels.NewSelector())
	if err != nil {
		return nil, errors.Wrap(err, "list namespace failed")
	}

	buf, err := generator.GenerateProjectConfig(projectLoggings, namespaces, systemProjectID)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func (s *ConfigGenerator) addExcludeNamespaces(systemProjectID string) (string, error) {
	namespaces, err := s.namespaceLister.List(metav1.NamespaceAll, labels.NewSelector())
	if err != nil {
		return "", errors.Wrapf(err, "list namespace failed")
	}

	var systemNamespaces []string
	for _, v := range namespaces {
		if v.Annotations[project.ProjectIDAnn] == systemProjectID {
			systemNamespaces = append(systemNamespaces, v.Name)
		}
	}

	return strings.Join(systemNamespaces, "|"), nil
}
