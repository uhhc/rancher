# rke

Rancher Kubernetes Engine, an extremely simple, lightning fast Kubernetes installer that works everywhere.

## Download

Please check the [releases](https://github.com/rancher/rke/releases/) page.

## Requirements

- Docker versions `1.11.2` up to `1.13.1` and `17.03.x` are validated for Kubernetes versions 1.8, 1.9 and 1.10
- OpenSSH 7.0+ must be installed on each node for stream local forwarding to work.
- The SSH user used for node access must be a member of the `docker` group:

```bash
usermod -aG docker <user_name>
```

- Ports 6443, 2379, and 2380 should be opened between cluster nodes.
- Swap disabled on worker nodes.

## Getting Started

Starting out with RKE? Check out this [blog post](http://rancher.com/an-introduction-to-rke/) or the [Quick Start Guide](https://github.com/rancher/rke/wiki/Quick-Start-Guide)

## Using RKE

Standing up a Kubernetes is as simple as creating a `cluster.yml` configuration file and running the command:

```bash
./rke up --config cluster.yml
```

### Full `cluster.yml` example

You can view full sample of cluster.yml [here](https://github.com/rancher/rke/blob/master/cluster.yml).

### Minimal `cluster.yml` example

```yaml
# default k8s version: v1.8.10-rancher1-1
# default network plugin: canal
nodes:
  - address: 1.2.3.4
    user: ubuntu
    role: [controlplane,worker,etcd]

```

## Network Plugins

RKE supports the following network plugins:

- Flannel
- Calico
- Canal
- Weave

To use specific network plugin configure `cluster.yml` to include:

```yaml
network:
  plugin: flannel
```

### Network Options

There are extra options that can be specified for each network plugin:

#### Flannel

- **flannel_image**: Flannel daemon Docker image
- **flannel_cni_image**: Flannel CNI binary installer Docker image
- **flannel_iface**: Interface to use for inter-host communication

#### Calico

- **calico_node_image**: Calico Daemon Docker image
- **calico_cni_image**: Calico CNI binary installer Docker image
- **calico_controllers_image**: Calico Controller Docker image
- **calicoctl_image**: Calicoctl tool Docker image
- **calico_cloud_provider**: Cloud provider where Calico will operate, currently supported values are: `aws`, `gce`

#### Canal

- **canal_node_image**: Canal Node Docker image
- **canal_cni_image**: Canal CNI binary installer Docker image
- **canal_flannel_image**: Canal Flannel Docker image

#### Weave

- **weave_node_image**: Weave Node Docker image
- **weave_cni_image**: Weave CNI binary installer Docker image

### RKE System Images

Prior to version `0.1.6`, RKE used the following list of images for deployment and cluster configuration:
```
system_images:
  etcd: rancher/etcd:v3.0.17
  kubernetes: rancher/k8s:v1.8.9-rancher1-1
  alpine: alpine:latest
  nginx_proxy: rancher/rke-nginx-proxy:v0.1.1
  cert_downloader: rancher/rke-cert-deployer:v0.1.1
  kubernetes_services_sidecar: rancher/rke-service-sidekick:v0.1.0
  kubedns: rancher/k8s-dns-kube-dns-amd64:1.14.5
  dnsmasq: rancher/k8s-dns-dnsmasq-nanny-amd64:1.14.5
  kubedns_sidecar: rancher/k8s-dns-sidecar-amd64:1.14.5
  kubedns_autoscaler: rancher/cluster-proportional-autoscaler-amd64:1.0.0
  flannel: rancher/coreos-flannel:v0.9.1
  flannel_cni: rancher/coreos-flannel-cni:v0.2.0
```
As of version `0.1.6`, we consolidated several of those images into a single image to simplify and speed the deployment process.

The following images are no longer required, and can be replaced by `rancher/rke-tools:v0.1.2`:
- alpine:latest
- rancher/rke-nginx-proxy:v0.1.1
- rancher/rke-cert-deployer:v0.1.1
- rancher/rke-service-sidekick:v0.1.0

## Addons

RKE supports pluggable addons on cluster bootstrap, user can specify the addon yaml in the cluster.yml file, and when running

```yaml
rke up --config cluster.yml
```

RKE will deploy the addons yaml after the cluster starts, RKE first uploads this yaml file as a configmap in kubernetes cluster and then run a kubernetes job that mounts this config map and deploy the addons.

> Note that RKE doesn't support yet removal of the addons, so once they are deployed the first time you can't change them using rke

To start using addons use `addons:` option in the `cluster.yml` file for example:

```yaml
addons: |-
    ---
    apiVersion: v1
    kind: Pod
    metadata:
      name: my-nginx
      namespace: default
    spec:
      containers:
      - name: my-nginx
        image: nginx
        ports:
        - containerPort: 80
```

Note that we are using `|-` because the addons option is a multi line string option, where you can specify multiple yaml files and separate them with `---`

For `addons_include:` you may pass either http/https urls or file paths, for example:
```yaml
addons_include:
    - https://raw.githubusercontent.com/rook/rook/master/cluster/examples/kubernetes/rook-operator.yaml
    - https://raw.githubusercontent.com/rook/rook/master/cluster/examples/kubernetes/rook-cluster.yaml
    - /opt/manifests/example.yaml
    - ./nginx.yaml
```

## High Availability

RKE is HA ready, you can specify more than one controlplane host in the `cluster.yml` file, and rke will deploy master components on all of them, the kubelets are configured to connect to `127.0.0.1:6443` by default which is the address of `nginx-proxy` service that proxy requests to all master nodes.

to start an HA cluster, just specify more than one host with role `controlplane`, and start the cluster normally.

## Adding/Removing Nodes

RKE supports adding/removing nodes for worker and controlplane hosts, in order to add additional nodes you will only need to update the `cluster.yml` file with additional nodes and run `rke up` with the same file.

To remove nodes just remove them from the hosts list in the cluster configuration file `cluster.yml`, and re run `rke up` command.

## Cluster Remove

RKE supports `rke remove` command, the command does the following:

- Connect to each host and remove the kubernetes services deployed on it.
- Clean each host from the directories left by the services:
  - /etc/kubernetes/ssl
  - /var/lib/etcd
  - /etc/cni
  - /opt/cni
  - /var/run/calico

Note that this command is irreversible and will destroy the kubernetes cluster entirely.

## Cluster Upgrade

RKE supports kubernetes cluster upgrade through changing the image version of services, in order to do that change the image option for each services, for example:

```yaml
image: rancher/k8s:v1.8.2-rancher1
```

TO

```yaml
image: rancher/k8s:v1.8.3-rancher2
```

And then run:

```bash
rke up --config cluster.yml
```

RKE will first look for the local `kube_config_cluster.yml` and then tries to upgrade each service to the latest image.

> Note that rollback isn't supported in RKE and may lead to unxpected results

## Service Upgrade

Service can also be upgraded by changing any of the services arguments or extra args and run `rke up` again with the updated configuration file.

> Please note that changing the following arguments: `service_cluster_ip_range` or `cluster_cidr` will result in a broken cluster, because currently the network pods will not be automatically upgraded.

## RKE Config

RKE supports command `rke config` which generates a cluster config template for the user, to start using this command just write:

```bash
rke config --name mycluster.yml
```

RKE will ask some questions around the cluster file like number of the hosts, ips, ssh users, etc, `--empty` option will generate an empty cluster.yml file, also if you just want to print on the screen and not save it in a file you can use `--print`.

## Ingress Controller

RKE will deploy Nginx controller by default, user can disable this by specifying `none` to ingress `provider` option in the cluster configuration, user also can specify list of options for nginx config map listed in this [doc](https://github.com/kubernetes/ingress-nginx/blob/master/docs/user-guide/configmap.md), and command line extra_args listed in this [doc](https://github.com/kubernetes/ingress-nginx/blob/master/docs/user-guide/cli-arguments.md), for example:
```
ingress:
  provider: nginx
  options:
    map-hash-bucket-size: "128"
    ssl-protocols: SSLv2
  extra_args:
    enable-ssl-passthrough: ""
```
By default, RKE will deploy ingress controller on all schedulable nodes (controlplane and workers), to specify only certain nodes for ingress controller to be deployed, user has to specify `node_selector` for the ingress and the right label on the node, for example:
```
nodes:
  - address: 1.1.1.1
    role: [controlplane,worker,etcd]
    user: root
    labels:
      app: ingress

ingress:
  provider: nginx
  node_selector:
    app: ingress
```

RKE will deploy Nginx Ingress controller as a DaemonSet with `hostnetwork: true`, so ports `80`, and `443` will be opened on each node where the controller is deployed.

## Extra Args and Binds

RKE supports additional service arguments.

```yaml
services:
  # ...
  kube-controller:
    extra_args:
      cluster-name: "mycluster"
```
This will add/append `--cluster-name=mycluster` to the container list of arguments.

As of `v0.1.3-rc2` using `extra_args` will add new arguments and **override** existing defaults. For example, if you need to modify the default admission controllers list, you need to change the default list and add apply it using `extra_args`.

RKE also supports additional volume binds:

```yaml
services:
  # ...
  kubelet:
    extra_binds:
      - "/host/dev:/dev"
      - "/usr/libexec/kubernetes/kubelet-plugins:/usr/libexec/kubernetes/kubelet-plugins:z"
```

## Authentication

RKE Supports x509 authentication strategy. You can additionally define a list of SANs (Subject Alternative Names) to add to the Kubernetes API Server PKI certificates. This allows you to connect to your Kubernetes cluster API Server through a load balancer, for example, rather than a single node.

```yaml
authentication:
  strategy: x509
  sans:
  - "10.18.160.10"
  - "my-loadbalancer-1234567890.us-west-2.elb.amazonaws.com"
```

## External etcd

RKE supports using external etcd instead of deploying etcd servers, to enable external etcd the following parameters should be populated:

```
services:
  etcd:
    path: /etcdcluster
    external_urls:
      - https://etcd-example.com:2379
    ca_cert: |-
      -----BEGIN CERTIFICATE-----
      xxxxxxxxxx
      -----END CERTIFICATE-----
    cert: |-
      -----BEGIN CERTIFICATE-----
      xxxxxxxxxx
      -----END CERTIFICATE-----
    key: |-
      -----BEGIN PRIVATE KEY-----
      xxxxxxxxxx
      -----END PRIVATE KEY-----
```

Note that RKE only supports connecting to TLS enabled etcd setup, user can enable multiple endpoints in the `external_urls` field. RKE will not accept having external urls and nodes with `etcd` role at the same time, user should only specify either etcd role for servers or external etcd but not both.

## Cloud Providers

Starting from v0.1.3 rke supports cloud providers.

### AWS Cloud Provider

To enable AWS cloud provider, you can set the following in the cluster configuration file:
```
cloud_provider:
  name: aws
```

AWS cloud provider has to be enabled on ec2 instances with the right IAM role.

### Azure Cloud provider

Azure cloud provider can be enabled by passing `azure` as the cloud provider name and set of options to the configuration file:
```
cloud_provider:
  name: azure
  cloud_config:
    aadClientId: xxxxxxxxxxxx
    aadClientSecret: xxxxxxxxxxx
    location: westus
    resourceGroup: rke-rg
    subnetName: rke-subnet
    subscriptionId: xxxxxxxxxxx
    vnetName: rke-vnet
    tenantId: xxxxxxxxxx
    securityGroupName: rke-nsg
```

You also have to make sure that the Azure node name must match the kubernetes node name, you can do that by changing the value of hostname_override in the config file:
```
nodes:
  - address: x.x.x.x
    hostname_override: azure-rke1
    user: ubuntu
    role:
    - controlplane
    - etcd
    - worker
```

## Deploying Rancher 2.0 using rke
Using RKE's pluggable user addons, it's possible to deploy Rancher 2.0 server in HA with a single command.

Depending how you want to manage your ssl certificates, there are 2 deployment options:

- Use own ssl cerficiates:
  - Use [rancher-minimal-ssl.yml](https://github.com/rancher/rke/blob/master/rancher-minimal-ssl.yml)
  - Update `nodes` configuration.
  - Update <FQDN> at `cattle-ingress-http` ingress definition. FQDN should be a dns a entry pointing to all nodes IP's running ingress-controller (controlplane and workers by default).
  - Update certificate, key and ca crt at `cattle-keys-server` secret, <BASE64_CRT>, <BASE64_KEY> and <BASE64_CA>. Content must be in base64 format, `cat <FILE> | base64`
  - Update ssl certificate and key at `cattle-keys-ingress` secret, <BASE64_CRT> and <BASE64_KEY>. Content must be in base64 format, `cat <FILE> | base64`. If selfsigned, certificate and key must be signed by same CA.  
  - Run RKE.

  ```bash
  rke up --config rancher-minimal-ssl.yml
  ```

- Use SSL-passthrough:
  - Use [rancher-minimal-passthrough.yml](https://github.com/rancher/rke/blob/master/rancher-minimal-passthrough.yml)
  - Update `nodes` configuration.
  - Update FQDN at `cattle-ingress-http` ingress definition. FQDN should be a dns a entry, pointing to all nodes IP's running ingress-controller (controlplane and workers by default).
  - Run RKE.

  ```bash
  rke up --config rancher-minimal-passthrough.yml
  ```

Once RKE execution finish, rancher is deployed at `cattle-system` namespace. You could access to your rancher instance by `https://<FQDN>`

By default, rancher deployment has just 1 replica, scale it to desired replicas.

```
kubectl -n cattle-system scale deployment cattle --replicas=3
```

## Operating Systems Notes

### Atomic OS

- Container volumes may have some issues in Atomic OS due to SELinux, most of volumes are mounted in rke with option `z`, however user still need to run the following commands before running rke:
```
# mkdir /opt/cni /etc/cni
# chcon -Rt svirt_sandbox_file_t /etc/cni
# chcon -Rt svirt_sandbox_file_t /opt/cni
```
- OpenSSH 6.4 shipped by default on Atomic CentOS which doesn't support SSH tunneling and therefore breaks rke, upgrading OpenSSH to the latest version supported by Atomic host will solve this problem:
```
# atomic host upgrade
```
- Atomic host doesn't come with docker group by default, you can change ownership of docker.sock to enable specific user to run rke:
```
# chown <user> /var/run/docker.sock
```

## License

Copyright (c) 2018 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
