package logging

import (
	"context"

	"github.com/rancher/rancher/pkg/controllers/user/logging/configsyncer"
	"github.com/rancher/rancher/pkg/controllers/user/logging/deployer"
	"github.com/rancher/rancher/pkg/controllers/user/logging/watcher"
	"github.com/rancher/types/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Register(ctx context.Context, cluster *config.UserContext) {

	clusterName := cluster.ClusterName
	secretManager := configsyncer.NewSecretManager(cluster)

	clusterLogging := cluster.Management.Management.ClusterLoggings(clusterName)
	projectLogging := cluster.Management.Management.ProjectLoggings(metav1.NamespaceAll)

	deployer := deployer.NewDeployer(cluster, secretManager)
	clusterLogging.AddClusterScopedHandler(ctx, "cluster-logging-deployer", cluster.ClusterName, deployer.ClusterLoggingSync)
	projectLogging.AddClusterScopedHandler(ctx, "project-logging-deployer", cluster.ClusterName, deployer.ProjectLoggingSync)

	configSyncer := configsyncer.NewConfigSyncer(cluster, secretManager)
	clusterLogging.AddClusterScopedHandler(ctx, "cluster-logging-configsyncer", cluster.ClusterName, configSyncer.ClusterLoggingSync)
	projectLogging.AddClusterScopedHandler(ctx, "project-logging-configsyncer", cluster.ClusterName, configSyncer.ProjectLoggingSync)

	namespaces := cluster.Core.Namespaces(metav1.NamespaceAll)
	namespaces.AddClusterScopedHandler(ctx, "namespace-logging-configsysncer", cluster.ClusterName, configSyncer.NamespaceSync)

	watcher.StartEndpointWatcher(ctx, cluster)
}
