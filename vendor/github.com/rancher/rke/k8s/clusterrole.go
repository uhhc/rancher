package k8s

import (
	"bytes"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
)

func UpdateClusterRoleBindingFromYaml(k8sClient *kubernetes.Clientset, clusterRoleBindingYaml string) error {
	clusterRoleBinding := rbacv1.ClusterRoleBinding{}
	decoder := yamlutil.NewYAMLToJSONDecoder(bytes.NewReader([]byte(clusterRoleBindingYaml)))
	err := decoder.Decode(&clusterRoleBinding)
	if err != nil {
		return err
	}

	for retries := 0; retries <= 5; retries++ {
		if _, err = k8sClient.RbacV1().ClusterRoleBindings().Create(&clusterRoleBinding); err != nil {
			if apierrors.IsAlreadyExists(err) {
				if _, err = k8sClient.RbacV1().ClusterRoleBindings().Update(&clusterRoleBinding); err == nil {
					return nil
				}
			}
		} else {
			return nil
		}
		time.Sleep(time.Second * 5)
	}
	return err
}

func UpdateClusterRoleFromYaml(k8sClient *kubernetes.Clientset, clusterRoleYaml string) error {
	clusterRole := rbacv1.ClusterRole{}
	err := decodeYamlResource(&clusterRole, clusterRoleYaml)
	if err != nil {
		return err
	}

	for retries := 0; retries <= 5; retries++ {
		if err = updateClusterRole(k8sClient, clusterRole); err != nil {
			time.Sleep(time.Second * 5)
			continue
		}
		return nil
	}
	return err
}

func updateClusterRole(k8sClient *kubernetes.Clientset, cr rbacv1.ClusterRole) error {
	if _, err := k8sClient.RbacV1().ClusterRoles().Create(&cr); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		if _, err := k8sClient.RbacV1().ClusterRoles().Update(&cr); err != nil {
			return err
		}
	}
	return nil
}
