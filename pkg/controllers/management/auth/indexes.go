package auth

import (
	"k8s.io/api/rbac/v1"
)

func rbByOwner(obj interface{}) ([]string, error) {
	rb, ok := obj.(*v1.RoleBinding)
	if !ok {
		return []string{}, nil
	}

	return getRBOwnerKey(rb), nil
}

func getRBOwnerKey(rb *v1.RoleBinding) []string {
	var owners []string
	for _, o := range rb.OwnerReferences {
		owners = append(owners, string(o.UID))
	}
	return owners
}

func rbRoleSubjectKey(roleName string, subject v1.Subject) string {
	return roleName + "." + subject.Kind + "." + subject.Name

}

func rbRoleSubjectKeys(roleName string, subjects []v1.Subject) []string {
	var keys []string
	for _, s := range subjects {
		keys = append(keys, rbRoleSubjectKey(roleName, s))
	}
	return keys
}

func rbByRoleAndSubject(obj interface{}) ([]string, error) {
	var subjects []v1.Subject
	var roleName string

	if rb, ok := obj.(*v1.ClusterRoleBinding); ok {
		roleName = rb.RoleRef.Name
		subjects = rb.Subjects
	} else if rb, ok := obj.(*v1.RoleBinding); ok {
		roleName = rb.RoleRef.Name
		subjects = rb.Subjects
	} else {
		return []string{}, nil
	}

	return rbRoleSubjectKeys(roleName, subjects), nil
}
