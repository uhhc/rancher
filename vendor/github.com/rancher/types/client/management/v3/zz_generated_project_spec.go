package client

const (
	ProjectSpecType               = "projectSpec"
	ProjectSpecFieldClusterId     = "clusterId"
	ProjectSpecFieldDescription   = "description"
	ProjectSpecFieldDisplayName   = "displayName"
	ProjectSpecFieldResourceQuota = "resourceQuota"
)

type ProjectSpec struct {
	ClusterId     string                `json:"clusterId,omitempty" yaml:"clusterId,omitempty"`
	Description   string                `json:"description,omitempty" yaml:"description,omitempty"`
	DisplayName   string                `json:"displayName,omitempty" yaml:"displayName,omitempty"`
	ResourceQuota *ProjectResourceQuota `json:"resourceQuota,omitempty" yaml:"resourceQuota,omitempty"`
}
