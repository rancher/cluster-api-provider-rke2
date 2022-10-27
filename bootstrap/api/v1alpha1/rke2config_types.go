/*
Copyright 2022 SUSE.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RKE2ConfigSpec defines the desired state of RKE2Config.
type RKE2ConfigSpec struct {
	// Files specifies extra files to be passed to user_data upon creation.
	//+optional
	Files []File `json:"files,omitempty"`

	// PreRKE2Commands specifies extra commands to run before rke2 setup runs.
	//+optional
	PreRKE2Commands []string `json:"preRKE2Commands,omitempty"`

	// PostRKE2Commands specifies extra commands to run after rke2 setup runs.
	//+optional
	PostRKE2Commands []string `json:"postRKE2Commands,omitempty"`

	// AgentConfig specifies configuration for the agent nodes.
	//+optional
	AgentConfig RKE2AgentConfig `json:"agentConfig,omitempty"`

	// ServerConfig specifies configuration for the agent nodes.
	//+optional
	ServerConfig RKE2ServerConfig `json:"serverConfig,omitempty"`

	// PrivateRegistriesConfig defines the containerd configuration for private registries and local registry mirrors.
	//+optional
	PrivateRegistriesConfig Registry `json:"privateRegistriesConfig,omitempty"`

	// Version specifies the rke2 version.
	//+optional
	Version string `json:"version,omitempty"`
}

type RKE2ServerConfig struct {
	// RKE2CommonNodeConfig is an inline struct that references common attribtes between agent and server nodes
	RKE2CommonNodeConfig `json:",inline"`

	// BindAddress describes the rke2 bind address (default: 0.0.0.0).
	//+optional
	BindAddress string `json:"bindAddress,omitempty"`

	// AdvertiseAddress IP address that apiserver uses to advertise to members of the cluster (default: node-external-ip/node-ip).
	//+optional
	AdvertiseAddress string `json:"advertiseAddress,omitempty"`

	// TLSSan Add additional hostname or IP as a Subject Alternative Name in the TLS cert.
	//+optional
	TLSSan []string `json:"tlsSan,omitempty"`

	// ServiceNodePortRange is the port range to reserve for services with NodePort visibility (default: "30000-32767").
	//+optional
	ServiceNodePortRange string `json:"service-node-port-range,omitempty"`

	// ClusterDNS is the cluster IP for CoreDNS service. Should be in your service-cidr range (default: 10.43.0.10).
	//+optional
	ClusterDNS string `json:"clusterDNS,omitempty"`

	// ClusterDomain is the cluster domain name (default: "cluster.local").
	//+optional
	ClusterDomain string `json:"clusterDomain,omitempty"`

	// ExposeEtcdMetrics defines the policy for ETCD Metrics exposure.
	// if value is true, ETCD metrics will be exposed
	// if value is false, ETCD metrics will NOT be exposed
	// +optional
	ExposeEtcdMetrics bool `json:"exposeEtcdMetrics,omitempty"`

	// EtcdBackupConfig defines how RKE2 will snapshot ETCD: target storage, schedule, etc.
	//+optional
	EtcdBackupConfig EtcdBackupConfig `json:"etcdBackupConfig,omitempty"`

	// DisableComponents lists Kubernetes components and RKE2 plugin components that will be disabled.
	//+optional
	DisableComponents DisableComponents `json:"disableComponents,omitempty"`

	// LoadBalancerPort Local port for supervisor client load-balancer. If the supervisor and apiserver are not colocated an additional port 1 less than this port will also be used for the apiserver client load-balancer (default: 6444).
	//+optional
	LoadBalancerPort int `json:"lbServerPort,omitempty"`

	// CNI describes the CNI Plugins to deploy, one of none, calico, canal, cilium; optionally with multus as the first value to enable the multus meta-plugin (default: canal).
	// +kubebuilder:validation:Enum=none;calico;canal;cilium
	//+optional
	CNI CNI `json:"cni,omitempty"`

	// PauseImage Override image to use for pause.
	//+optional
	PauseImage string `json:"pauseImage,omitempty"`

	// RuntimeImage Override image to use for runtime binaries (containerd, kubectl, crictl, etc).
	//+optional
	RuntimeImage string `json:"runtimeImage,omitempty"`

	//	CloudProviderName  Cloud provider name.
	//+optional
	CloudProviderName string `json:"cloudProviderName,omitempty"`

	//	CloudProviderConfigMap  is a reference to a ConfigMap containing Cloud provider configuration.
	//+optional
	CloudProviderConfigMap corev1.ObjectReference `json:"cloudProviderConfigMap,omitempty"`

	// NOTE: this was only profile, changed it to cisProfile.

	// AuditPolicySecret Path to the file that defines the audit policy configuration.
	//+optional
	AuditPolicySecret corev1.ObjectReference `json:"auditPolicySecret,omitempty"`

	// ControlPlaneResourceRequests Control Plane resource requests.
	//+optional
	ControlPlaneResourceRequests string `json:"controlPlaneResourceRequests,omitempty"`

	// ControlPlaneResourceLimits Control Plane resource limits.
	//+optional
	ControlPlaneResourceLimits string `json:"controlPlaneResourceLimits,omitempty"`

	// Etcd defines optional custom configuration of ETCD.
	//+optional
	Etcd ComponentConfig `json:"etcd,omitempty"`

	// KubeAPIServer defines optional custom configuration of the Kube API Server.
	//+optional
	KubeAPIServer ComponentConfig `json:"kubeAPIServer,omitempty"`

	// KubeControllerManager defines optional custom configuration of the Kube Controller Manager.
	//+optional
	KubeControllerManager ComponentConfig `json:"kubeControllerManager,omitempty"`

	// KubeScheduler defines optional custom configuration of the Kube Scheduler.
	//+optional
	KubeScheduler ComponentConfig `json:"kubeScheduler,omitempty"`

	// CloudControllerManager defines optional custom configuration of the Cloud Controller Manager.
	//+optional
	CloudControllerManager ComponentConfig `json:"cloudControllerManager,omitempty"`
}

type RKE2AgentConfig struct {
	// RKE2CommonNodeConfig is an inline struct that references common attribtes between agent and server nodes
	RKE2CommonNodeConfig `json:",inline"`
}

// RKE2CommonNodeConfig describes some attributes that are common to agent and server nodes
type RKE2CommonNodeConfig struct {
	// DataDir Folder to hold state.
	//+optional
	DataDir string `json:"dataDir,omitempty"`

	// NodeLabels  Registering and starting kubelet with set of labels.
	//+optional
	NodeLabels []string `json:"nodeLabels,omitempty"`

	// NodeTaints Registering kubelet with set of taints.
	//+optional
	NodeTaints []string `json:"nodeTaints,omitempty"`

	// NodeNamePrefix Prefix to the Node Name that CAPI will generate.
	//+optional
	NodeNamePrefix string `json:"nodeName,omitempty"`

	// ImageCredentialProviderConfigMap is a reference to the ConfigMap that contains credential provider plugin config
	// The configMap should contain a YAML file content + a Path to the Binaries for Credential Provider.
	//+optional
	ImageCredentialProviderConfigMap corev1.ObjectReference `json:"imageCredentialProviderConfigMap,omitempty"`

	// TODO: Remove ContainerRuntimeEndpoint since this feature will probably not be offered by CAPI Bootstrap provider?

	// ContainerRuntimeEndpoint Disable embedded containerd and use alternative CRI implementation.
	//+optional
	ContainerRuntimeEndpoint string `json:"containerRuntimeEndpoint,omitempty"`

	// TODO: Find a way to handle IP addresses that should be advertised but that RKE2 cannot find on the host (Example: Elastic IPs on Cloud Providers).

	// NodeIp IPv4/IPv6 addresses to advertise for node.
	//+optional.
	//NodeIp string `json:"nodeIp,omitempty"`

	// NodeExternalIp IPv4/IPv6 external IP addresses to advertise for node.
	//+optional
	// NodeExternalIp string `json:"nodeExternalIp,omitempty"`

	// CISProfile activates CIS compliance of RKE2 for a certain profile
	// +kubebuilder:validation:Enum=cis-1.23
	//+optional
	CISProfile CISProfile `json:"cisProfile,omitempty"`

	// ResolvConf is a reference to a ConfigMap containing resolv.conf content for the node.
	//+optional
	ResolvConf corev1.ObjectReference `json:"resolvConf,omitempty"`

	// ProtectKernelDefaults defines Kernel tuning behavior. If true, error if kernel tunables are different than kubelet defaults.
	// if false, kernel tunable can be different from kubelet defaults
	//+optional
	ProtectKernelDefaults bool `json:"protectKernelDefaults,omitempty"`

	// SystemDefaultRegistry Private registry to be used for all system images.
	//+optional
	SystemDefaultRegistry string `json:"systemDefaultRegistry,omitempty"`

	// EnableContainerdSElinux defines the policy for enabling SELinux for Containerd
	// if value is true, Containerd will run with selinux-enabled=true flag
	// if value is false, Containerd will run without the above flag
	//+optional
	EnableContainerdSElinux bool `json:"enableContainerdSElinux,omitempty"`

	// KubeletPath Override kubelet binary path.
	//+optional
	KubeletPath string `json:"kubeletPath,omitempty"`

	// KubeletArgs Customized flag for kubelet process.
	//+optional
	Kubelet ComponentConfig `json:"kubelet,omitempty"`

	// KubeProxyArgs Customized flag for kube-proxy process.
	//+optional
	KubeProxy ComponentConfig `json:"kubeProxy,omitempty"`
}

// DisableComponents describes components of RKE2 (Kubernetes components and plugin components) that should be disabled
type DisableComponents struct {
	// KubernetesComponents is a list of Kubernetes components to disable.
	// +kubebuilder:validation:Enum=scheduler;kubeProxy;cloudController
	KubernetesComponents []DisabledKubernetesComponent `json:"kubernetesComponents,omitempty"`

	// PluginComponents is a list of PluginComponents to disable.
	// +kubebuilder:validation:Enum=rke2-coredns;rke2-ingress-nginx;rke2-metrics-server
	PluginComponents []DisabledPluginComponent `json:"pluginComponents,omitempty"`
}

// RKE2ConfigStatus defines the observed state of RKE2Config.
type RKE2ConfigStatus struct {
	// Ready indicates the BootstrapData field is ready to be consumed.
	Ready bool `json:"ready,omitempty"`

	// DataSecretName is the name of the secret that stores the bootstrap data script.
	//+optional
	DataSecretName *string `json:"dataSecretName,omitempty"`

	// FailureReason will be set on non-retryable errors.
	//+optional
	FailureReason string `json:"failureReason,omitempty"`

	// FailureMessage will be set on non-retryable errors.
	//+optional
	FailureMessage string `json:"failureMessage,omitempty"`

	// ObservedGeneration is the latest generation observed by the controller.
	//+optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions defines current service state of the RKE2Config.
	//+optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RKE2Config is the Schema for the rke2configs API.
type RKE2Config struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RKE2ConfigSpec   `json:"spec,omitempty"`
	Status RKE2ConfigStatus `json:"status,omitempty"`
}

func (c *RKE2Config) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

func (c *RKE2Config) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// RKE2ConfigList contains a list of RKE2Config.
type RKE2ConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RKE2Config `json:"items"`
}

// DisabledItem selects a plugin Components to be disabled.
type DisabledPluginComponent string

const (
	// CoreDNS references the RKE2 Plugin "rke2-coredns"
	CoreDNS DisabledPluginComponent = "rke2-coredns"
	// IngressNginx references the RKE2 Plugin "rke2-ingress-nginx"
	IngressNginx DisabledPluginComponent = "rke2-ingress-nginx"
	// MetricsServer references the RKE2 Plugin "rke2-metrics-server"
	MetricsServer DisabledPluginComponent = "rke2-metrics-server"
)

// CISProfile defines the CIS Benchmark profile to be activated in RKE2.
type CISProfile string

const (
	// CIS1_23 references RKE2's CIS Profile "cis-1.23"
	CIS1_23 CISProfile = "cis-1.23"
)

// CNI defines the Cni options for deploying RKE2.
type CNI string

const (
	// Cilium references the RKE2 CNI Plugin "cilium"
	Cilium CNI = "cilium"
	// Calico references the RKE2 CNI Plugin "calico"
	Calico CNI = "calico"
	// Canal references the RKE2 CNI Plugin "canal"
	Canal CNI = "canal"
	// None means that no CNI Plugin will be installed with RKE2, letting the operator install his own CNI afterwards.
	None CNI = "none"
)

// Encoding specifies the cloud-init file encoding.
type Encoding string

const (
	// Base64 implies the contents of the file are encoded as base64.
	Base64 Encoding = "base64"
	// Gzip implies the contents of the file are encoded with gzip.
	Gzip Encoding = "gzip"
	// GzipBase64 implies the contents of the file are first base64 encoded and then gzip encoded.
	GzipBase64 Encoding = "gzip+base64"
)

// DisabledKubernetesComponent is an enum field that can take one of the following values: scheduler, kubeProxy or cloudController.
type DisabledKubernetesComponent string

const (
	// Scheduler references the Kube Scheduler Kubernetes components of the control plane/server nodes
	Scheduler DisabledKubernetesComponent = "scheduler"

	// KubeProxy references the Kube Proxy Kubernetes components on the agents
	KubeProxy DisabledKubernetesComponent = "kubeProxy"

	// CloudController references the Cloud Controller Manager Kubernetes Components on the control plane / server nodes
	CloudController DisabledKubernetesComponent = "cloudController"
)

// File defines the input for generating write_files in cloud-init.
type File struct {
	// Path specifies the full path on disk where to store the file.
	Path string `json:"path"`

	// Owner specifies the ownership of the file, e.g. "root:root".
	//+optional
	Owner string `json:"owner,omitempty"`

	// Permissions specifies the permissions to assign to the file, e.g. "0640".
	//+optional
	Permissions string `json:"permissions,omitempty"`

	// Encoding specifies the encoding of the file contents.
	// +kubebuilder:validation:Enum=base64;gzip;gzip+base64
	//+optional
	Encoding Encoding `json:"encoding,omitempty"`

	// Content is the actual content of the file.
	//+optional
	Content string `json:"content,omitempty"`

	// ContentFrom is a referenced source of content to populate the file.
	//+optional
	ContentFrom *FileSource `json:"contentFrom,omitempty"`
}

// FileSource is a union of all possible external source types for file data.
// Only one field may be populated in any given instance. Developers adding new
// sources of data for target systems should add them here.
type FileSource struct {
	// Secret represents a secret that should populate this file.
	Secret SecretFileSource `json:"secret"`
}

// Adapts a Secret into a FileSource.
//
// The contents of the target Secret's Data field will be presented
// as files using the keys in the Data field as the file names.
type SecretFileSource struct {
	// Name of the secret in the RKE2BootstrapConfig's namespace to use.
	Name string `json:"name"`

	// Key is the key in the secret's data map for this value.
	Key string `json:"key"`
}

type EtcdBackupConfig struct {
	// EnableAutomaticSnapshots defines the policy for ETCD snapshots. true means automatic snapshots will be scheduled, false means automatic snapshots will not be scheduled.
	//+optional
	EnableAutomaticSnapshots bool `json:"enableAutomaticSnapshots,omitempty"`

	// SnapshotName Set the base name of etcd snapshots. Default: etcd-snapshot-<unix-timestamp> (default: "etcd-snapshot").
	//+optional
	SnapshotName string `json:"snapshotName,omitempty"`

	// ScheduleCron Snapshot interval time in cron spec. eg. every 5 hours '* */5 * * *' (default: "0 */12 * * *").
	//+optional
	ScheduleCron string `json:"scheduleCron,omitempty"`

	// Retention Number of snapshots to retain Default: 5 (default: 5).
	//+optional
	Retention string `json:"retention,omitempty"`

	// Directory Directory to save db snapshots. (Default location: ${data-dir}/db/snapshots).
	//+optional
	Directory string `json:"directory,omitempty"`

	// S3 Enable backup to an S3-compatible Object Store.
	//+optional
	S3 EtcdS3 `json:"s3,omitempty"`
}

// Registry is registry settings including mirrors, TLS, and credentials.
type Registry struct {
	// Mirrors are namespace to mirror mapping for all namespaces.
	//+optional
	Mirrors map[string]Mirror `json:"mirrors,omitempty"`

	// Configs are configs for each registry.
	// The key is the FDQN or IP of the registry.
	//+optional
	Configs map[string]RegistryConfig `json:"configs,omitempty"`
}

// Mirror contains the config related to the registry mirror.
type Mirror struct {
	// Endpoints are endpoints for a namespace. CRI plugin will try the endpoints
	// one by one until a working one is found. The endpoint must be a valid url
	// with host specified.
	// The scheme, host and path from the endpoint URL will be used.
	//+optional
	Endpoints []string `json:"endpoint,omitempty"`

	// Rewrites are repository rewrite rules for a namespace. When fetching image resources
	// from an endpoint and a key matches the repository via regular expression matching
	// it will be replaced with the corresponding value from the map in the resource request.
	//+optional
	Rewrites map[string]string `json:"rewrite,omitempty"`
}

// RegistryConfig contains configuration used to communicate with the registry.
type RegistryConfig struct {
	// Auth si a reference to a Secret containing information to authenticate to the registry.
	// The Secret must provite a username and a password data entry.
	//+optional
	AuthSecret corev1.ObjectReference `json:"authSecret,omitempty"`
	// TLS is a pair of CA/Cert/Key which then are used when creating the transport
	// that communicates with the registry.
	//+optional
	TLS TLSConfig `json:"tls,omitempty"`
}

// TLSConfig contains the CA/Cert/Key used for a registry.
type TLSConfig struct {
	// TLSConfigSecret is a reference to a secret of type `kubernetes.io/tls` thich has up to 3 entries: tls.crt, tls.key and ca.crt
	// which describe the TLS configuration necessary to connect to the registry.
	// +optional
	TLSConfigSecret corev1.ObjectReference `json:"tlsConfigSecret,omitempty"`

	// EnforceSSLVerify may be set to false to skip verifying the registry's certificate, default is true.
	//+optional
	EnforceSSLVerify bool `json:"enforceSslVerify,omitempty"`
}

type EtcdS3 struct {
	// Endpoint S3 endpoint url (default: "s3.amazonaws.com").
	Endpoint string `json:"endpoint"`

	// EndpointCA references the Secret that contains a custom CA that should be trusted to connect to S3 endpoint.
	//+optional
	EndpointCA corev1.ObjectReference `json:"endpointCA,omitempty"`

	// EnforceSSLVerify may be set to false to skip verifying the registry's certificate, default is true.
	//+optional
	EnforceSSLVerify bool `json:"enforceSslVerify,omitempty"`

	// S3CredentialSecret is a reference to a Secret containing the Access Key and Secret Key necessary to access the target S3 Bucket.
	S3CredentialSecret corev1.ObjectReference `json:"S3CredentialSecret"`

	// Bucket S3 bucket name.
	//+optional
	Bucket string `json:"bucket,omitempty"`

	// Region S3 region / bucket location (optional) (default: "us-east-1").
	//+optional
	Region string `json:"region,omitempty"`

	// Folder S3 folder.
	//+optional
	Folder string `json:"folder,omitempty"`
}

type ComponentConfig struct {
	// ExtraEnv is a map of environment variables to pass on to a Kubernetes Component command.
	//+optional
	ExtraEnv map[string]string `json:"extraEnv,omitempty"`

	// ExtraArgs is a map of command line arguments to pass to a Kubernetes Component command.
	//+optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	//ExtraMounts is a map of volume mounts to be added for the Kubernetes component StaticPod
	//+optional
	ExtraMounts map[string]string `json:"extraMounts,omitempty"`

	//OverrideImage is a string that references a container image to override the default one for the Kubernetes Component
	//+optional
	OverrideImage string `json:"overrideImage,omitempty"`
}

func init() {
	SchemeBuilder.Register(&RKE2Config{}, &RKE2ConfigList{})
}
