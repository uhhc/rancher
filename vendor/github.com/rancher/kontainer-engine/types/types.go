package types

import (
	"context"
)

const (
	// StringType is the type for string flag
	StringType = "string"
	// BoolType is the type for bool flag
	BoolType = "bool"
	// IntType is the type for int flag
	IntType = "int"
	// StringSliceType is the type for stringSlice flag
	StringSliceType = "stringSlice"
)

// Driver defines the interface that each driver plugin should implement
type Driver interface {
	// GetDriverCreateOptions returns cli flags that are used in create
	GetDriverCreateOptions(ctx context.Context) (*DriverFlags, error)

	// GetDriverUpdateOptions returns cli flags that are used in update
	GetDriverUpdateOptions(ctx context.Context) (*DriverFlags, error)

	// Create creates the cluster. clusterInfo is only set when we are retrying a failed or interrupted create
	Create(ctx context.Context, opts *DriverOptions, clusterInfo *ClusterInfo) (*ClusterInfo, error)

	// Update updates the cluster
	Update(ctx context.Context, clusterInfo *ClusterInfo, opts *DriverOptions) (*ClusterInfo, error)

	// PostCheck does post action after provisioning
	PostCheck(ctx context.Context, clusterInfo *ClusterInfo) (*ClusterInfo, error)

	// Remove removes the cluster
	Remove(ctx context.Context, clusterInfo *ClusterInfo) error

	GetVersion(ctx context.Context, clusterInfo *ClusterInfo) (*KubernetesVersion, error)
	SetVersion(ctx context.Context, clusterInfo *ClusterInfo, version *KubernetesVersion) error
	GetClusterSize(ctx context.Context, clusterInfo *ClusterInfo) (*NodeCount, error)
	SetClusterSize(ctx context.Context, clusterInfo *ClusterInfo, count *NodeCount) error

	// Get driver capabilities
	GetCapabilities(ctx context.Context) (*Capabilities, error)
}

type UnimplementedVersionAccess struct {
}

func (u *UnimplementedVersionAccess) GetVersion(ctx context.Context, info *ClusterInfo) (*KubernetesVersion, error) {
	return nil, nil
}

func (u *UnimplementedVersionAccess) SetVersion(ctx context.Context, info *ClusterInfo, version *KubernetesVersion) error {
	return nil

}

type UnimplementedClusterSizeAccess struct {
}

func (u *UnimplementedClusterSizeAccess) GetClusterSize(ctx context.Context, info *ClusterInfo) (*NodeCount, error) {
	return nil, nil

}

func (u *UnimplementedClusterSizeAccess) SetClusterSize(ctx context.Context, info *ClusterInfo, count *NodeCount) error {
	return nil

}
