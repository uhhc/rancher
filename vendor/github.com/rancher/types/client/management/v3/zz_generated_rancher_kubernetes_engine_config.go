package client

const (
	RancherKubernetesEngineConfigType                     = "rancherKubernetesEngineConfig"
	RancherKubernetesEngineConfigFieldAddons              = "addons"
	RancherKubernetesEngineConfigFieldAuthentication      = "authentication"
	RancherKubernetesEngineConfigFieldAuthorization       = "authorization"
	RancherKubernetesEngineConfigFieldIgnoreDockerVersion = "ignoreDockerVersion"
	RancherKubernetesEngineConfigFieldIngress             = "ingress"
	RancherKubernetesEngineConfigFieldNetwork             = "network"
	RancherKubernetesEngineConfigFieldNodes               = "nodes"
	RancherKubernetesEngineConfigFieldPrivateRegistries   = "privateRegistries"
	RancherKubernetesEngineConfigFieldSSHKeyPath          = "sshKeyPath"
	RancherKubernetesEngineConfigFieldServices            = "services"
	RancherKubernetesEngineConfigFieldVersion             = "kubernetesVersion"
)

type RancherKubernetesEngineConfig struct {
	Addons              string             `json:"addons,omitempty" yaml:"addons,omitempty"`
	Authentication      *AuthnConfig       `json:"authentication,omitempty" yaml:"authentication,omitempty"`
	Authorization       *AuthzConfig       `json:"authorization,omitempty" yaml:"authorization,omitempty"`
	IgnoreDockerVersion bool               `json:"ignoreDockerVersion,omitempty" yaml:"ignoreDockerVersion,omitempty"`
	Ingress             *IngressConfig     `json:"ingress,omitempty" yaml:"ingress,omitempty"`
	Network             *NetworkConfig     `json:"network,omitempty" yaml:"network,omitempty"`
	Nodes               []RKEConfigNode    `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	PrivateRegistries   []PrivateRegistry  `json:"privateRegistries,omitempty" yaml:"privateRegistries,omitempty"`
	SSHKeyPath          string             `json:"sshKeyPath,omitempty" yaml:"sshKeyPath,omitempty"`
	Services            *RKEConfigServices `json:"services,omitempty" yaml:"services,omitempty"`
	Version             string             `json:"kubernetesVersion,omitempty" yaml:"kubernetesVersion,omitempty"`
}
