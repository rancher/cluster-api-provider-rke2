# RKE2 Cluster API Provider - Data Type definitions

## Introduction

The Cluster API Bootstrap provider for RKE2 has a goal of provisioning RKE2 on a Cluster API machine. It relies on `cloud-init` to provision files, modify configuration and run commands on the machine.

The idea is that the provider gives the user a large choice of configuration options, but uses as many sensible default as possible to avoid overburdening the user.

Therefore a particular attention has to be given to the kinds of manifests the end user should write. This document aims at documenting the process with which the data types for these manifests have been thought through.

## Configuration options

RKE2 is a very configurable Kubernetes distribution. The main ways to configure RKE2 are as follows:

- config.yaml file (default location at /etc/rancher/rke2/): configuration options for RKE2 that are described in this [documentation page]([Server Configuration Reference - RKE2 - Rancher's Next Generation Kubernetes Distribution](https://docs.rke2.io/install/install_options/server_config/))
  
- registries.yaml ()
  
- Environement variables for versions, etc. (options documented [here]([Overview - RKE2 - Rancher's Next Generation Kubernetes Distribution](https://docs.rke2.io/install/install_options/install_options/#configuring-the-linux-installation-script)))
  
- Possibly automatically deploy manifests in `/var/lib/rancher/rke2/server/manifests/`
  
- Should be possible to deploy in **Air-Gapped** mode
  

<mark>Question: Should the use be able to uninstall ?</mark>

### First configuration section: config.yaml

In order to make RKE2 installation sufficiently configurable, we rely on the documentation page above and implement all options.

This is what the page shows for RKE2 **<u>servers</u>**:

```
   --config FILE, -c FILE                        (config) Load configuration from FILE (default: "/etc/rancher/rke2/config.yaml") [$RKE2_CONFIG_FILE]
   --debug                                       (logging) Turn on debug logs [$RKE2_DEBUG]
   --bind-address value                          (listener) rke2 bind address (default: 0.0.0.0)
   --advertise-address value                     (listener) IPv4 address that apiserver uses to advertise to members of the cluster (default: node-external-ip/node-ip)
   --tls-san value                               (listener) Add additional hostnames or IPv4/IPv6 addresses as Subject Alternative Names on the server TLS cert
   --data-dir value, -d value                    (data) Folder to hold state (default: "/var/lib/rancher/rke2")
   --cluster-cidr value                          (networking) IPv4/IPv6 network CIDRs to use for pod IPs (default: 10.42.0.0/16)
   --service-cidr value                          (networking) IPv4/IPv6 network CIDRs to use for service IPs (default: 10.43.0.0/16)
   --service-node-port-range value               (networking) Port range to reserve for services with NodePort visibility (default: "30000-32767")
   --cluster-dns value                           (networking) IPv4 Cluster IP for coredns service. Should be in your service-cidr range (default: 10.43.0.10)
   --cluster-domain value                        (networking) Cluster Domain (default: "cluster.local")
   --token value, -t value                       (cluster) Shared secret used to join a server or agent to a cluster [$RKE2_TOKEN]
   --token-file value                            (cluster) File containing the cluster-secret/token [$RKE2_TOKEN_FILE]
   --write-kubeconfig value, -o value            (client) Write kubeconfig for admin client to this file [$RKE2_KUBECONFIG_OUTPUT]
   --write-kubeconfig-mode value                 (client) Write kubeconfig with this mode [$RKE2_KUBECONFIG_MODE]
   --kube-apiserver-arg value                    (flags) Customized flag for kube-apiserver process
   --etcd-arg value                              (flags) Customized flag for etcd process
   --kube-controller-manager-arg value           (flags) Customized flag for kube-controller-manager process
   --kube-scheduler-arg value                    (flags) Customized flag for kube-scheduler process
   --etcd-expose-metrics                         (db) Expose etcd metrics to client interface. (Default false)
   --etcd-disable-snapshots                      (db) Disable automatic etcd snapshots
   --etcd-snapshot-name value                    (db) Set the base name of etcd snapshots. Default: etcd-snapshot-<unix-timestamp> (default: "etcd-snapshot")
   --etcd-snapshot-schedule-cron value           (db) Snapshot interval time in cron spec. eg. every 5 hours '* */5 * * *' (default: "0 */12 * * *")
   --etcd-snapshot-retention value               (db) Number of snapshots to retain Default: 5 (default: 5)
   --etcd-snapshot-dir value                     (db) Directory to save db snapshots. (Default location: ${data-dir}/db/snapshots)
   --etcd-s3                                     (db) Enable backup to S3
   --etcd-s3-endpoint value                      (db) S3 endpoint url (default: "s3.amazonaws.com")
   --etcd-s3-endpoint-ca value                   (db) S3 custom CA cert to connect to S3 endpoint
   --etcd-s3-skip-ssl-verify                     (db) Disables S3 SSL certificate validation
   --etcd-s3-access-key value                    (db) S3 access key [$AWS_ACCESS_KEY_ID]
   --etcd-s3-secret-key value                    (db) S3 secret key [$AWS_SECRET_ACCESS_KEY]
   --etcd-s3-bucket value                        (db) S3 bucket name
   --etcd-s3-region value                        (db) S3 region / bucket location (optional) (default: "us-east-1")
   --etcd-s3-folder value                        (db) S3 folder
   --disable value                               (components) Do not deploy packaged components and delete any deployed components (valid items: rke2-coredns, rke2-ingress-nginx, rke2-metrics-server)
   --disable-scheduler                           (components) Disable Kubernetes default scheduler
   --disable-cloud-controller                    (components) Disable rke2 default cloud controller manager
   --disable-kube-proxy                          (components) Disable running kube-proxy
   --node-name value                             (agent/node) Node name [$RKE2_NODE_NAME]
   --node-label value                            (agent/node) Registering and starting kubelet with set of labels
   --node-taint value                            (agent/node) Registering kubelet with set of taints
   --image-credential-provider-bin-dir value     (agent/node) The path to the directory where credential provider plugin binaries are located (default: "/var/lib/rancher/credentialprovider/bin")
   --image-credential-provider-config value      (agent/node) The path to the credential provider plugin config file (default: "/var/lib/rancher/credentialprovider/config.yaml")
   --container-runtime-endpoint value            (agent/runtime) Disable embedded containerd and use alternative CRI implementation
   --snapshotter value                           (agent/runtime) Override default containerd snapshotter (default: "overlayfs")
   --private-registry value                      (agent/runtime) Private registry configuration file (default: "/etc/rancher/rke2/registries.yaml")
   --node-ip value, -i value                     (agent/networking) IPv4/IPv6 addresses to advertise for node
   --node-external-ip value                      (agent/networking) IPv4/IPv6 external IP addresses to advertise for node
   --resolv-conf value                           (agent/networking) Kubelet resolv.conf file [$RKE2_RESOLV_CONF]
   --kubelet-arg value                           (agent/flags) Customized flag for kubelet process
   --kube-proxy-arg value                        (agent/flags) Customized flag for kube-proxy process
   --protect-kernel-defaults                     (agent/node) Kernel tuning behavior. If set, error if kernel tunables are different than kubelet defaults.
   --agent-token value                           (experimental/cluster) Shared secret used to join agents to the cluster, but not servers [$RKE2_AGENT_TOKEN]
   --agent-token-file value                      (experimental/cluster) File containing the agent secret [$RKE2_AGENT_TOKEN_FILE]
   --server value, -s value                      (experimental/cluster) Server to connect to, used to join a cluster [$RKE2_URL]
   --cluster-reset                               (experimental/cluster) Forget all peers and become sole member of a new cluster [$RKE2_CLUSTER_RESET]
   --cluster-reset-restore-path value            (db) Path to snapshot file to be restored
   --system-default-registry value               (image) Private registry to be used for all system images [$RKE2_SYSTEM_DEFAULT_REGISTRY]
   --selinux                                     (agent/node) Enable SELinux in containerd [$RKE2_SELINUX]
   --lb-server-port value                        (agent/node) Local port for supervisor client load-balancer. If the supervisor and apiserver are not colocated an additional port 1 less than this port will also be used for the apiserver client load-balancer. (default: 6444) [$RKE2_LB_SERVER_PORT]
   --cni value                                   (networking) CNI Plugins to deploy, one of none, calico, canal, cilium; optionally with multus as the first value to enable the multus meta-plugin (default: canal) [$RKE2_CNI]
   --kube-apiserver-image value                  (image) Override image to use for kube-apiserver [$RKE2_KUBE_APISERVER_IMAGE]
   --kube-controller-manager-image value         (image) Override image to use for kube-controller-manager [$RKE2_KUBE_CONTROLLER_MANAGER_IMAGE]
   --kube-proxy-image value                      (image) Override image to use for kube-proxy [$RKE2_KUBE_PROXY_IMAGE]
   --kube-scheduler-image value                  (image) Override image to use for kube-scheduler [$RKE2_KUBE_SCHEDULER_IMAGE]
   --pause-image value                           (image) Override image to use for pause [$RKE2_PAUSE_IMAGE]
   --runtime-image value                         (image) Override image to use for runtime binaries (containerd, kubectl, crictl, etc) [$RKE2_RUNTIME_IMAGE]
   --etcd-image value                            (image) Override image to use for etcd [$RKE2_ETCD_IMAGE]
   --kubelet-path value                          (experimental/agent) Override kubelet binary path [$RKE2_KUBELET_PATH]
   --cloud-provider-name value                   (cloud provider) Cloud provider name [$RKE2_CLOUD_PROVIDER_NAME]
   --cloud-provider-config value                 (cloud provider) Cloud provider configuration file path [$RKE2_CLOUD_PROVIDER_CONFIG]
   --profile value                               (security) Validate system configuration against the selected benchmark (valid items: cis-1.23 ) [$RKE2_CIS_PROFILE]
   --audit-policy-file value                     (security) Path to the file that defines the audit policy configuration [$RKE2_AUDIT_POLICY_FILE]
   --control-plane-resource-requests value       (components) Control Plane resource requests [$RKE2_CONTROL_PLANE_RESOURCE_REQUESTS]
   --control-plane-resource-limits value         (components) Control Plane resource limits [$RKE2_CONTROL_PLANE_RESOURCE_LIMITS]
   --kube-apiserver-extra-mount value            (components) kube-apiserver extra volume mounts [$RKE2_KUBE_APISERVER_EXTRA_MOUNT]
   --kube-scheduler-extra-mount value            (components) kube-scheduler extra volume mounts [$RKE2_KUBE_SCHEDULER_EXTRA_MOUNT]
   --kube-controller-manager-extra-mount value   (components) kube-controller-manager extra volume mounts [$RKE2_KUBE_CONTROLLER_MANAGER_EXTRA_MOUNT]
   --kube-proxy-extra-mount value                (components) kube-proxy extra volume mounts [$RKE2_KUBE_PROXY_EXTRA_MOUNT]
   --etcd-extra-mount value                      (components) etcd extra volume mounts [$RKE2_ETCD_EXTRA_MOUNT]
   --cloud-controller-manager-extra-mount value  (components) cloud-controller-manager extra volume mounts [$RKE2_CLOUD_CONTROLLER_MANAGER_EXTRA_MOUNT]
   --kube-apiserver-extra-env value              (components) kube-apiserver extra environment variables [$RKE2_KUBE_APISERVER_EXTRA_ENV]
   --kube-scheduler-extra-env value              (components) kube-scheduler extra environment variables [$RKE2_KUBE_SCHEDULER_EXTRA_ENV]
   --kube-controller-manager-extra-env value     (components) kube-controller-manager extra environment variables [$RKE2_KUBE_CONTROLLER_MANAGER_EXTRA_ENV]
   --kube-proxy-extra-env value                  (components) kube-proxy extra environment variables [$RKE2_KUBE_PROXY_EXTRA_ENV]
   --etcd-extra-env value                        (components) etcd extra environment variables [$RKE2_ETCD_EXTRA_ENV]
   --cloud-controller-manager-extra-env value    (components) cloud-controller-manager extra environment variables [$RKE2_CLOUD_CONTROLLER_MANAGER_EXTRA_ENV]```
```

In order to transform that into a struct, we can use the following regex which catches each line with its formatting:

```regex
 *--([a-z0-9\-]*) (value(, \-[a-z] value){0,1}){0,1} *\([a-z/]*\) ([\u\l ,\d/\.\-]*)(.*$)
```

with the following for replacement:

```regex
// $1 $4\n//+optional\n$1\n$5\n\n
```

This will create a pseudo-struct definition that does not satisfy the Kubernetes API and Golang guidelines for attribute naming

### Filtering previous

- Token
  
- TokenFile
  

Are probably not needed since the token can be generated automatically and should not necessarily be known to/provided by the user. After some work on the attribute formatting and some clean up, we can get the first workable intermediate result.

### Intermediate result

This shows a first usable intermediate result:

```go
type RKE2ServerConfig struct {
	// Debug is boolean that turns on debug logs if true (default: false)
	//+optional
	Debug bool `json:"debug,omitempty"`

	// BindAddress describes the rke2 bind address (default: 0.0.0.0)
	// +optional
	BindAddress string `json:"bindAddress,omitempty"`

	// AdvertiseAddress IP address that apiserver uses to advertise to members of the cluster (default: node-external-ip/node-ip)
	// +optional
	AdvertiseAddress string `json:"advertiseAddress,omitempty"`

	// TLSSan Add additional hostname or IP as a Subject Alternative Name in the TLS cert
	// +optional
	TLSSan []string `json:"tlsSan,omitempty"`

	// DataDir is the Folder to hold RKE2's state (default: "/var/lib/rancher/rke2")
	//+optional
	DataDir string `json:"data-dir,omitempty"`

	// ClusterCidr  Network CIDR to use for pod IPs (default: "10.42.0.0/16")
	// +optional
	ClusterCidr string `json:"clusterCidr,omitempty"`

	// ServiceCidr Network CIDR to use for services IPs (default: "10.43.0.0/16")
	// +optional
	ServiceCidr string `json:"serviceCidr,omitempty"`

	// ServiceNodePortRange is the port range to reserve for services with NodePort visibility (default: "30000-32767")
	//+optional
	ServiceNodePortRange string `json:"service-node-port-range,omitempty"`

	// ClusterDNS  Cluster IP for coredns service. Should be in your service-cidr range (default: 10.43.0.10)
	// +optional
	ClusterDNS string `json:"clusterDNS,omitempty"`

	// ClusterDomain Cluster Domain (default: "cluster.local")
	// +optional
	ClusterDomain string `json:"clusterDomain,omitempty"`

	// TODO: Remove both Token and TokenFile attributes

	// token Shared secret used to join a server or agent to a cluster
	//+optional
	//Token	string	`json:"token,omitempty"`

	// token-file File containing the cluster-secret/token
	//+optional
	//TokenFile	string	`json:"token-file,omitempty"`

	// WriteKubeconfig path to which kubeconfig file for admin client will be written
	// +optional
	WriteKubeconfig string `json:"writeKubeconfig,omitempty"`

	// WriteKubeconfigMode Write kubeconfig with this mode
	// +optional
	WriteKubeconfigMode string `json:"writeKubeconfigMode,omitempty"`

	// KubeApiserverArgs Customized flag for kube-apiserver process
	// +optional
	KubeApiserverArgs []string `json:"kubeApiserverArgs,omitempty"`

	// EtcdArgs Customized flag for etcd process
	// +optional
	EtcdArgs []string `json:"etcdArgs,omitempty"`

	// KubeControllerManagerArgs Customized flag for kube-controller-manager process
	// +optional
	KubeControllerManagerArgs []string `json:"kubeControllerManagerArgs,omitempty"`

	// KubeSchedulerArgs Customized flag for kube-scheduler process
	// +optional
	KubeSchedulerArgs []string `json:"kubeSchedulerArgs,omitempty"`

	// EtcdExposeMetrics Expose etcd metrics to client interface. (Default false)
	// +optional
	EtcdExposeMetrics string `json:"etcdExposeMetrics,omitempty"`

	// EtcdDisableSnapshots Disable automatic etcd snapshots
	// +optional
	EtcdDisableSnapshots string `json:"etcdDisableSnapshots,omitempty"`

	// EtcdSnapshotName Set the base name of etcd snapshots. Default: etcd-snapshot-<unix-timestamp> (default: "etcd-snapshot")
	// +optional
	EtcdSnapshotName string `json:"etcdSnapshotName,omitempty"`

	// EtcdSnapshotScheduleCron Snapshot interval time in cron spec. eg. every 5 hours '* */5 * * *' (default: "0 */12 * * *")
	// +optional
	EtcdSnapshotScheduleCron string `json:"etcdSnapshotScheduleCron,omitempty"`

	// EtcdSnapshotRetention Number of snapshots to retain Default: 5 (default: 5)
	// +optional
	EtcdSnapshotRetention string `json:"etcdSnapshotRetention,omitempty"`

	// EtcdSnapshotDir Directory to save db snapshots. (Default location: ${data-dir}/db/snapshots)
	// +optional
	EtcdSnapshotDir string `json:"etcdSnapshotDir,omitempty"`

	// EtcdS3 Enable backup to S3
	// +optional
	EtcdS3 string `json:"etcdS3,omitempty"`

	// EtcdS3Endpoint S3 endpoint url (default: "s3.amazonaws.com")
	// +optional
	EtcdS3Endpoint string `json:"etcdS3Endpoint,omitempty"`

	// EtcdS3EndpointCa S3 custom CA cert to connect to S3 endpoint
	// +optional
	EtcdS3EndpointCa string `json:"etcdS3EndpointCa,omitempty"`

	// EtcdS3SkipSslVerify Disables S3 SSL certificate validation
	// +optional
	EtcdS3SkipSslVerify string `json:"etcdS3SkipSslVerify,omitempty"`

	// EtcdS3AccessKey S3 access key
	// +optional
	EtcdS3AccessKey string `json:"etcdS3AccessKey,omitempty"`

	// EtcdS3SecretKey S3 secret key
	// +optional
	EtcdS3SecretKey string `json:"etcdS3SecretKey,omitempty"`

	// EtcdS3Bucket S3 bucket name
	// +optional
	EtcdS3Bucket string `json:"etcdS3Bucket,omitempty"`

	// EtcdS3Region S3 region / bucket location (optional) (default: "us-east-1")
	// +optional
	EtcdS3Region string `json:"etcdS3Region,omitempty"`

	// EtcdS3Folder S3 folder
	// +optional
	EtcdS3Folder string `json:"etcdS3Folder,omitempty"`

	// Disable Do not deploy packaged components and delete any deployed components (valid items: rke2-coredns, rke2-ingress-nginx, rke2-metrics-server)
	// +optional
	Disable []DisabledItem `json:"disable,omitempty"`

	// DisableScheduler Disable Kubernetes default scheduler
	// +optional
	DisableScheduler string `json:"disable-scheduler,omitempty"`

	// DisableCloudController Disable rke2 default cloud controller manager
	// +optional
	DisableCloudController string `json:"disableCloudController,omitempty"`

	// DisableKubeProxy Disable running kube-proxy
	// +optional
	DisableKubeProxy string `json:"disableKubeProxy,omitempty"`

	// NodeName Node name
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// NodeLabel Registering and starting kubelet with set of labels
	// +optional
	NodeLabel string `json:"nodeLabel,omitempty"`

	// NodeTaint Registering kubelet with set of taints
	// +optional
	NodeTaint string `json:"nodeTaint,omitempty"`

	// ImageCredentialProviderBinDir The path to the directory where credential provider plugin binaries are located (default: "/var/lib/rancher/credentialprovider/bin")
	// +optional
	ImageCredentialProviderBinDir string `json:"imageCredentialProviderBinDir,omitempty"`

	// ImageCredentialProviderConfig The path to the credential provider plugin config file (default: "/var/lib/rancher/credentialprovider/config.yaml")
	// +optional
	ImageCredentialProviderConfig string `json:"imageCredentialProviderConfig,omitempty"`

	// ContainerRuntimeEndpoint Disable embedded containerd and use alternative CRI implementation
	// +optional
	ContainerRuntimeEndpoint string `json:"containerRuntimeEndpoint,omitempty"`

	// Snapshotter Override default containerd snapshotter (default: "overlayfs")
	// +optional
	Snapshotter string `json:"snapshotter,omitempty"`

	// TODO: Decide if user should be able to do this here, registries.yaml might integrated in ConfigSpec.

	// PrivateRegistry Private registry configuration file (default: "/etc/rancher/rke2/registries.yaml")
	// +optional
	PrivateRegistry string `json:"privateRegistry,omitempty"`

	// NodeIp IPv4/IPv6 addresses to advertise for node
	// +optional
	NodeIp string `json:"nodeIp,omitempty"`

	// NodeExternalIp IPv4/IPv6 external IP addresses to advertise for node
	// +optional
	NodeExternalIp string `json:"nodeExternalIp,omitempty"`

	// ResolvConf Kubelet resolv.conf file
	// +optional
	ResolvConf string `json:"resolvConf,omitempty"`

	// KubeletArgs Customized flag for kubelet process
	// +optional
	KubeletArgs []string `json:"kubeletArgs,omitempty"`

	// KubeProxyArgs Customized flag for kube-proxy process
	// +optional
	KubeProxyArgs []string `json:"kubeProxyArgs,omitempty"`

	// ProtectKernelDefaults Kernel tuning behavior. If set, error if kernel tunables are different than kubelet defaults.
	// +optional
	ProtectKernelDefaults string `json:"protectKernelDefaults,omitempty"`

	// AgentToken Shared secret used to join agents to the cluster, but not servers
	// +optional
	AgentToken string `json:"agentToken,omitempty"`

	// AgentTokenFile File containing the agent secret
	// +optional
	AgentTokenFile string `json:"agentTokenFile,omitempty"`

	// Server Server to connect to, used to join a cluster
	// +optional
	Server string `json:"server,omitempty"`

	// ClusterReset Forget all peers and become sole member of a new cluster
	// +optional
	ClusterReset string `json:"clusterReset,omitempty"`

	// ClusterResetRestorePath Path to snapshot file to be restored
	// +optional
	ClusterResetRestorePath string `json:"clusterResetRestorePath,omitempty"`

	// SystemDefaultRegistry Private registry to be used for all system images
	// +optional
	SystemDefaultRegistry string `json:"systemDefaultRegistry,omitempty"`

	// Selinux Enable SELinux in containerd
	// +optional
	Selinux string `json:"selinux,omitempty"`

	// LbServerPort Local port for supervisor client load-balancer. If the supervisor and apiserver are not colocated an additional port 1 less than this port will also be used for the apiserver client load-balancer. (default: 6444)
	// +optional
	LbServerPort string `json:"lbServerPort,omitempty"`

	// Cni CNI Plugins to deploy, one of none, calico, canal, cilium; optionally with multus as the first value to enable the multus meta-plugin (default: canal)
	// +optional
	Cni Cni `json:"cni,omitempty"`

	// KubeApiserverImage Override image to use for kube-apiserver
	// +optional
	KubeApiserverImage string `json:"kubeApiserverImage,omitempty"`

	// KubeControllerManagerImage Override image to use for kube-controller-manager
	// +optional
	KubeControllerManagerImage string `json:"kubeControllerManagerImage,omitempty"`

	// KubeProxyImage Override image to use for kube-proxy
	// +optional
	KubeProxyImage string `json:"kubeProxyImage,omitempty"`

	// KubeSchedulerImage Override image to use for kube-scheduler
	// +optional
	KubeSchedulerImage string `json:"kubeSchedulerImage,omitempty"`

	// PauseImage Override image to use for pause
	// +optional
	PauseImage string `json:"pauseImage,omitempty"`

	// RuntimeImage Override image to use for runtime binaries (containerd, kubectl, crictl, etc)
	// +optional
	RuntimeImage string `json:"runtimeImage,omitempty"`

	// EtcdImage Override image to use for etcd
	// +optional
	EtcdImage string `json:"etcdImage,omitempty"`

	// KubeletPath Override kubelet binary path
	// +optional
	KubeletPath string `json:"kubeletPath,omitempty"`

	//	CloudProviderName  Cloud provider name
	//
	// +optional
	CloudProviderName string `json:"cloudProviderName,omitempty"`

	//	CloudProviderConfig  Cloud provider configuration file path
	//
	// +optional
	CloudProviderConfig string `json:"cloudProviderConfig,omitempty"`

	// NOTE: this was only profile, changed it to cisProfile

	// CisProfile Validate system configuration against the selected benchmark (valid items: cis-1.23 )
	// +optional
	CisProfile CisProfile `json:"cisProfile,omitempty"`

	// AuditPolicyFile Path to the file that defines the audit policy configuration
	// +optional
	AuditPolicyFile string `json:"auditPolicyFile,omitempty"`

	// ControlPlaneResourceRequests Control Plane resource requests
	// +optional
	ControlPlaneResourceRequests string `json:"controlPlaneResourceRequests,omitempty"`

	// ControlPlaneResourceLimits Control Plane resource limits
	// +optional
	ControlPlaneResourceLimits string `json:"controlPlaneResourceLimits,omitempty"`

	// KubeApiserverExtraMount kube-apiserver extra volume mounts
	// +optional
	KubeApiserverExtraMount string `json:"kubeApiserverExtraMount,omitempty"`

	// KubeSchedulerExtraMount kube-scheduler extra volume mounts
	// +optional
	KubeSchedulerExtraMount string `json:"kubeSchedulerExtraMount,omitempty"`

	// KubeControllerManagerExtraMount kube-controller-manager extra volume mounts
	// +optional
	KubeControllerManagerExtraMount string `json:"kubeControllerManagerExtraMount,omitempty"`

	// KubeProxyExtraMount kube-proxy extra volume mounts
	// +optional
	KubeProxyExtraMount string `json:"kubeProxyExtraMount,omitempty"`

	// EtcdExtraMount etcd extra volume mounts
	// +optional
	EtcdExtraMount string `json:"etcdExtraMount,omitempty"`

	// CloudControllerManagerExtraMount cloud-controller-manager extra volume mounts
	// +optional
	CloudControllerManagerExtraMount string `json:"cloudControllerManagerExtraMount,omitempty"`

	// KubeApiserverExtraEnv kube-apiserver extra environment variables
	// +optional
	KubeApiserverExtraEnv string `json:"kubeApiserverExtraEnv,omitempty"`

	// KubeSchedulerExtraEnv kube-scheduler extra environment variables
	// +optional
	KubeSchedulerExtraEnv string `json:"kubeSchedulerExtraEnv,omitempty"`

	// KubeControllerManagerExtraEnv kube-controller-manager extra environment variables
	// +optional
	KubeControllerManagerExtraEnv string `json:"kubeControllerManagerExtraEnv,omitempty"`

	// KubeProxyExtraEnv kube-proxy extra environment variables
	// +optional
	KubeProxyExtraEnv string `json:"kubeProxyExtraEnv,omitempty"`

	// EtcdExtraEnv etcd extra environment variables
	// +optional
	EtcdExtraEnv string `json:"etcdExtraEnv,omitempty"`

	// CloudControllerManagerExtraEnv cloud-controller-manager extra environment variables
	// +optional
	CloudControllerManagerExtraEnv string `json:"cloudControllerManagerExtraEnv,omitempty"`
}


// DisabledItem selects a plugin Components to be disabled
// +kubebuilder:validation:enum=rke2-coredns;rke2-ingress-nginx;rke2-metrics-server
type DisabledItem string

// CisProfile defines the CIS Benchmark profile to be activated in RKE2
// +kubebuilder:validation:enum=cis-1.23
type CisProfile string

// Cni defines the Cni options for deploying RKE2
// +kubebuilder:validation:enum=none;calico;canal;cilium
type Cni string
```
