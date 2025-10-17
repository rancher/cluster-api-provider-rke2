/*
Copyright 2024 SUSE LLC.

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

package v1beta1

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
)

const (
	// RKE2ControlPlaneLegacyFinalizer allows the controller to clean up resources on delete.
	// this is the old finalizer name. It is kept to ensure backward compatibility.
	RKE2ControlPlaneLegacyFinalizer = "rke2.controleplane.cluster.x-k8s.io"
	// RKE2ControlPlaneFinalizer allows the controller to clean up resources on delete.
	RKE2ControlPlaneFinalizer = "rke2.controlplane.cluster.x-k8s.io"

	// RKE2ServerConfigurationAnnotation is a machine annotation that stores the json-marshalled string of RKE2Config
	// This annotation is used to detect any changes in RKE2Config and trigger machine rollout.
	RKE2ServerConfigurationAnnotation = "controlplane.cluster.x-k8s.io/rke2-server-configuration"

	// LegacyRKE2ControlPlane is a controlplane annotation that marks the CP as legacy. This CP will not provide
	// etcd certificate management or etcd membership management.
	LegacyRKE2ControlPlane = "controlplane.cluster.x-k8s.io/legacy"

	// RemediationInProgressAnnotation is used to keep track that a RCP remediation is in progress, and more
	// specifically it tracks that the system is in between having deleted an unhealthy machine and recreating its replacement.
	// NOTE: if something external to CAPI removes this annotation the system cannot detect the above situation; this can lead to
	// failures in updating remediation retry or remediation count (both counters restart from zero).
	RemediationInProgressAnnotation = "controlplane.cluster.x-k8s.io/remediation-in-progress"

	// RemediationForAnnotation is used to link a new machine to the unhealthy machine it is replacing;
	// please note that in case of retry, when also the remediating machine fails, the system keeps track of
	// the first machine of the sequence only.
	// NOTE: if something external to CAPI removes this annotation the system this can lead to
	// failures in updating remediation retry (the counter restarts from zero).
	RemediationForAnnotation = "controlplane.cluster.x-k8s.io/remediation-for"

	// DefaultMinHealthyPeriod defines the default minimum period before we consider a remediation on a
	// machine unrelated from the previous remediation.
	DefaultMinHealthyPeriod = 1 * time.Hour

	// LoadBalancerExclusionAnnotation is an annotation applicable to RKE2ControlPlanes, which will trigger the application
	// of the `node.kubernetes.io/exclude-from-external-load-balancers` label on control plane Nodes during Machine deletion.
	// This label can be consumed by load balancers to stop advertising a Node.
	LoadBalancerExclusionAnnotation = "rke2.controlplane.cluster.x-k8s.io/load-balancer-exclusion"
)

// RKE2ControlPlaneSpec defines the desired state of RKE2ControlPlane.
type RKE2ControlPlaneSpec struct {
	// RKE2AgentSpec contains the node spec for the RKE2 Control plane nodes.
	bootstrapv1.RKE2ConfigSpec `json:",inline"`

	// Replicas is the number of replicas for the Control Plane.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Version defines the desired Kubernetes version.
	// This field takes precedence over RKE2ConfigSpec.AgentConfig.Version (which is deprecated).
	// +kubebuilder:validation:Pattern="(v\\d\\.\\d{2}\\.\\d+\\+rke2r\\d)|^$"
	// +optional
	Version string `json:"version"`

	// MachineTemplate contains information about how machines
	// should be shaped when creating or updating a control plane.
	// +optional
	MachineTemplate RKE2ControlPlaneMachineTemplate `json:"machineTemplate,omitempty"`

	// ServerConfig specifies configuration for the agent nodes.
	//+optional
	ServerConfig RKE2ServerConfig `json:"serverConfig,omitempty"`

	// ManifestsConfigMapReference references a ConfigMap which contains Kubernetes manifests to be deployed automatically on the cluster
	// Each data entry in the ConfigMap will be will be copied to a folder on the control plane nodes that RKE2 scans and uses to deploy manifests.
	//+optional
	ManifestsConfigMapReference corev1.ObjectReference `json:"manifestsConfigMapReference,omitempty"`

	// InfrastructureRef is a required reference to a custom resource
	// offered by an infrastructure provider.
	// This field is deprecated. Use `.machineTemplate.infrastructureRef` instead.
	// +optional
	// +kubebuilder:deprecatedversion:warning="Use `.machineTemplate.infrastructureRef` instead"
	InfrastructureRef corev1.ObjectReference `json:"infrastructureRef"`

	// NodeDrainTimeout is the total amount of time that the controller will spend on draining a controlplane node
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// This field is deprecated. Use `.machineTemplate.nodeDrainTimeout` instead.
	// +optional
	// +kubebuilder:deprecatedversion:warning="Use `.machineTemplate.nodeDrainTimeout` instead"
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// RegistrationMethod is the method to use for registering nodes into the RKE2 cluster.
	// +kubebuilder:validation:Enum=internal-first;internal-only-ips;external-only-ips;address;control-plane-endpoint;""
	// +optional
	RegistrationMethod RegistrationMethod `json:"registrationMethod,omitempty"`

	// RegistrationAddress is an explicit address to use when registering a node. This is required if
	// the registration type is "address". Its for scenarios where a load-balancer or VIP is used.
	// +optional
	RegistrationAddress string `json:"registrationAddress,omitempty"`

	// The RolloutStrategy to use to replace control plane machines with new ones.
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy"`

	// remediationStrategy is the RemediationStrategy that controls how control plane machine remediation happens.
	// +optional
	RemediationStrategy *RemediationStrategy `json:"remediationStrategy,omitempty"`
}

// RKE2ControlPlaneMachineTemplate defines the template for Machines
// in a RKE2ControlPlane object.
type RKE2ControlPlaneMachineTemplate struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// InfrastructureRef is a required reference to a custom resource
	// offered by an infrastructure provider.
	InfrastructureRef corev1.ObjectReference `json:"infrastructureRef"`

	// NodeDrainTimeout is the total amount of time that the controller will spend on draining a controlplane node
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// nodeDeletionTimeout defines how long the machine controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// If no value is provided, the default value for this property of the Machine resource will be used.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`
}

// RKE2ServerConfig specifies configuration for the agent nodes.
type RKE2ServerConfig struct {
	// AuditPolicySecret path to the file that defines the audit policy configuration.
	//+optional
	AuditPolicySecret *corev1.ObjectReference `json:"auditPolicySecret,omitempty"`

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
	ServiceNodePortRange string `json:"serviceNodePortRange,omitempty"`

	// ClusterDNS is the cluster IP for CoreDNS service. Should be in your service-cidr range (default: 10.43.0.10).
	//+optional
	ClusterDNS string `json:"clusterDNS,omitempty"`

	// ClusterDomain is the cluster domain name (default: "cluster.local").
	//+optional
	ClusterDomain string `json:"clusterDomain,omitempty"`

	// DisableComponents lists Kubernetes components and RKE2 plugin components that will be disabled.
	//+optional
	DisableComponents DisableComponents `json:"disableComponents,omitempty"`

	// CNI describes the CNI Plugins to deploy, one of none, calico, canal, cilium;
	// optionally with multus as the first value to enable the multus meta-plugin (default: canal).
	// +kubebuilder:validation:Enum=none;calico;canal;cilium
	//+optional
	CNI CNI `json:"cni,omitempty"`

	// CNIMultusEnable enables multus as the first CNI plugin (default: false).
	// This option will automatically make Multus a primary CNI, and the value, if specified in the CNI field, as a secondary CNI plugin.
	//+optional
	CNIMultusEnable bool `json:"cniMultusEnable,omitempty"`

	// PauseImage Override image to use for pause.
	//+optional
	PauseImage string `json:"pauseImage,omitempty"`

	// Etcd defines optional custom configuration of ETCD.
	//+optional
	Etcd EtcdConfig `json:"etcd,omitempty"`

	// SecretsEncrytion defines encryption at rest configuration
	//+optional
	SecretsEncryptionProvider *SecretsEncryption `json:"secretsEncryption,omitempty"`

	// KubeAPIServer defines optional custom configuration of the Kube API Server.
	//+optional
	KubeAPIServer *bootstrapv1.ComponentConfig `json:"kubeAPIServer,omitempty"`

	// KubeControllerManager defines optional custom configuration of the Kube Controller Manager.
	//+optional
	KubeControllerManager *bootstrapv1.ComponentConfig `json:"kubeControllerManager,omitempty"`

	// KubeScheduler defines optional custom configuration of the Kube Scheduler.
	//+optional
	KubeScheduler *bootstrapv1.ComponentConfig `json:"kubeScheduler,omitempty"`

	// CloudControllerManager defines optional custom configuration of the Cloud Controller Manager.
	//+optional
	CloudControllerManager *bootstrapv1.ComponentConfig `json:"cloudControllerManager,omitempty"`

	// CloudProviderName cloud provider name.
	//+optional
	CloudProviderName string `json:"cloudProviderName,omitempty"`
	// CloudProviderConfigMap is a reference to a ConfigMap containing Cloud provider configuration.
	// The config map must contain a key named cloud-config.
	//+optional
	CloudProviderConfigMap *corev1.ObjectReference `json:"cloudProviderConfigMap,omitempty"`

	// EmbeddedRegistry enables the embedded registry.
	//+optional
	EmbeddedRegistry bool `json:"embeddedRegistry,omitempty"`

	// ExternalDatastoreSecret is a reference to a Secret that contains configuration about connecting to an external datastore.
	// The secret must contain a key named "endpoint" that contains the connection string for the external datastore.
	// +optional
	ExternalDatastoreSecret *corev1.ObjectReference `json:"externalDatastoreSecret,omitempty"`
}

// RKE2ControlPlaneStatus defines the observed state of RKE2ControlPlane.
type RKE2ControlPlaneStatus struct {
	// Ready denotes that the RKE2ControlPlane API Server became ready during initial provisioning
	// to receive requests.
	// NOTE: this field is part of the Cluster API contract and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed. Please use conditions
	// to check the operational state of the control plane.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Initialized indicates the target cluster has completed initialization.
	Initialized bool `json:"initialized,omitempty"`

	// DataSecretName is the name of the secret that stores the bootstrap data script.
	// +optional
	DataSecretName *string `json:"dataSecretName,omitempty"`

	// FailureReason will be set on non-retryable errors.
	// +optional
	FailureReason string `json:"failureReason,omitempty"`

	// FailureMessage will be set on non-retryable errors.
	// +optional
	FailureMessage string `json:"failureMessage,omitempty"`

	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions defines current service state of the RKE2Config.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// Replicas is the number of replicas current attached to this ControlPlane Resource.
	Replicas int32 `json:"replicas,omitempty"`

	// Version represents the minimum Kubernetes version for the control plane machines
	// in the cluster.
	// +optional
	Version *string `json:"version,omitempty"`

	// ReadyReplicas is the number of replicas current attached to this ControlPlane Resource and that have Ready Status.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// UpdatedReplicas is the number of replicas current attached to this ControlPlane Resource and that are up-to-date with Control Plane config.
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// UnavailableReplicas is the number of replicas current attached to this ControlPlane Resource and that are up-to-date with Control Plane config.
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// AvailableServerIPs is a list of the Control Plane IP adds that can be used to register further nodes.
	// +optional
	AvailableServerIPs []string `json:"availableServerIPs,omitempty"`

	// lastRemediation stores info about last remediation performed.
	// +optional
	LastRemediation *LastRemediationStatus `json:"lastRemediation,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// RKE2ControlPlane is the Schema for the rke2controlplanes API.
type RKE2ControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RKE2ControlPlaneSpec   `json:"spec,omitempty"`
	Status RKE2ControlPlaneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RKE2ControlPlaneList contains a list of RKE2ControlPlane.
type RKE2ControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RKE2ControlPlane `json:"items"`
}

// EtcdConfig regroups the ETCD-specific configuration of the control plane.
type EtcdConfig struct {
	// ExposeEtcdMetrics defines the policy for ETCD Metrics exposure.
	// if value is true, ETCD metrics will be exposed
	// if value is false, ETCD metrics will NOT be exposed
	// +optional
	ExposeMetrics bool `json:"exposeMetrics,omitempty"`

	// BackupConfig defines how RKE2 will snapshot ETCD: target storage, schedule, etc.
	//+optional
	BackupConfig EtcdBackupConfig `json:"backupConfig,omitempty"`

	// CustomConfig defines the custom settings for ETCD.
	CustomConfig *bootstrapv1.ComponentConfig `json:"customConfig,omitempty"`
}

// EtcdBackupConfig describes the backup configuration for ETCD.
type EtcdBackupConfig struct {
	// DisableAutomaticSnapshots defines the policy for ETCD snapshots.
	// true means automatic snapshots will be scheduled, false means automatic snapshots will not be scheduled.
	//+optional
	DisableAutomaticSnapshots *bool `json:"disableAutomaticSnapshots,omitempty"`

	// SnapshotName Set the base name of etcd snapshots. Default: etcd-snapshot-<unix-timestamp> (default: "etcd-snapshot").
	//+optional
	SnapshotName string `json:"snapshotName,omitempty"`

	// ScheduleCron Snapshot interval time in cron spec. eg. every 5 hours '* */5 * * *' (default: "0 */12 * * *").
	//+optional
	ScheduleCron string `json:"scheduleCron,omitempty"`

	// Retention Number of snapshots to retain Default: 5 (default: 5).
	//+optional
	Retention string `json:"retention,omitempty"`

	// Directory to save db snapshots.
	//+optional
	Directory string `json:"directory,omitempty"`

	// S3 Enable backup to an S3-compatible Object Store.
	//+optional
	S3 *EtcdS3 `json:"s3,omitempty"`
}

// EtcdS3 defines the S3 configuration for ETCD snapshots.
type EtcdS3 struct {
	// Endpoint S3 endpoint url (default: "s3.amazonaws.com").
	Endpoint string `json:"endpoint"`

	// EndpointCA references the Secret that contains a custom CA that should be trusted to connect to S3 endpoint.
	// The secret must contain a key named "ca.pem" that contains the CA certificate.
	//+optional
	EndpointCASecret *corev1.ObjectReference `json:"endpointCAsecret,omitempty"`

	// EnforceSSLVerify may be set to false to skip verifying the registry's certificate, default is true.
	//+optional
	EnforceSSLVerify bool `json:"enforceSslVerify,omitempty"`

	// S3CredentialSecret is a reference to a Secret containing the Access Key and Secret Key necessary to access the target S3 Bucket.
	// The Secret must contain the following keys: "aws_access_key_id" and "aws_secret_access_key".
	// If empty, the controller will default to IAM authentication
	S3CredentialSecret *corev1.ObjectReference `json:"s3CredentialSecret,omitempty"`

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

// SecretsEncryption defines encryption configuration.
type SecretsEncryption struct {
	// EncyptionKey secret reference
	EncryptionKeySecret *corev1.ObjectReference `json:"encryptionKeySecret,omitempty"`
	// Encryption provider
	// +kubebuilder:validation:Enum=aescbc;secretbox
	Provider string `json:"provider,omitempty"`
}

// CNI defines the Cni options for deploying RKE2.
type CNI string

const (
	// Cilium references the RKE2 CNI Plugin "cilium".
	Cilium CNI = "cilium"
	// Calico references the RKE2 CNI Plugin "calico".
	Calico CNI = "calico"
	// Canal references the RKE2 CNI Plugin "canal".
	Canal CNI = "canal"
	// None means that no CNI Plugin will be installed with RKE2, letting the operator install his own CNI afterwards.
	None CNI = "none"
)

// DisableComponents describes components of RKE2 (Kubernetes components and plugin components) that should be disabled.
type DisableComponents struct {
	// KubernetesComponents is a list of Kubernetes components to disable.
	KubernetesComponents []DisabledKubernetesComponent `json:"kubernetesComponents,omitempty"`

	// PluginComponents is a list of PluginComponents to disable.
	PluginComponents []DisabledPluginComponent `json:"pluginComponents,omitempty"`
}

// DisabledKubernetesComponent is an enum field that can take one of the following values: scheduler, kubeProxy or cloudController.
// +kubebuilder:validation:Enum=scheduler;kubeProxy;cloudController
type DisabledKubernetesComponent string

const (
	// Scheduler references the Kube Scheduler Kubernetes components of the control plane/server nodes.
	Scheduler DisabledKubernetesComponent = "scheduler"

	// KubeProxy references the Kube Proxy Kubernetes components on the agents.
	KubeProxy DisabledKubernetesComponent = "kubeProxy"

	// CloudController references the Cloud Controller Manager Kubernetes Components on the control plane / server nodes.
	CloudController DisabledKubernetesComponent = "cloudController"
)

//+kubebuilder:validation:Enum=rke2-coredns;rke2-ingress-nginx;rke2-metrics-server;rke2-snapshot-controller;rke2-snapshot-controller-crd;rke2-snapshot-validation-webhook

// DisabledPluginComponent selects a plugin Components to be disabled.
type DisabledPluginComponent string

const (
	// CoreDNS references the RKE2 Plugin "rke2-coredns".
	CoreDNS DisabledPluginComponent = "rke2-coredns"
	// IngressNginx references the RKE2 Plugin "rke2-ingress-nginx".
	IngressNginx DisabledPluginComponent = "rke2-ingress-nginx"
	// MetricsServer references the RKE2 Plugin "rke2-metrics-server".
	MetricsServer DisabledPluginComponent = "rke2-metrics-server"
	// SnapshotController references the RKE2 Plugin "rke2-snapshot-controller".
	SnapshotController DisabledPluginComponent = "rke2-snapshot-controller"
	// SnapshotControllerCRD references the RKE2 Plugin "rke2-snapshot-controller-crd".
	SnapshotControllerCRD DisabledPluginComponent = "rke2-snapshot-controller-crd"
	// SnapshotValidationWebhook references the RKE2 Plugin "rke2-snapshot-validation-webhook".
	SnapshotValidationWebhook DisabledPluginComponent = "rke2-snapshot-validation-webhook"
)

// RemediationStrategy allows to define how control plane machine remediation happens.
type RemediationStrategy struct {
	// maxRetry is the Max number of retries while attempting to remediate an unhealthy machine.
	// A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
	// For example, given a control plane with three machines M1, M2, M3:
	//
	//	M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.
	//	If M1-1 (replacement of M1) has problems while bootstrapping it will become unhealthy, and then be
	//	remediated; such operation is considered a retry, remediation-retry #1.
	//	If M1-2 (replacement of M1-1) becomes unhealthy, remediation-retry #2 will happen, etc.
	//
	// A retry could happen only after RetryPeriod from the previous retry.
	// If a machine is marked as unhealthy after MinHealthyPeriod from the previous remediation expired,
	// this is not considered a retry anymore because the new issue is assumed unrelated from the previous one.
	//
	// If not set, the remedation will be retried infinitely.
	// +optional
	MaxRetry *int32 `json:"maxRetry,omitempty"`

	// retryPeriod is the duration that RKE2ControlPlane should wait before remediating a machine being created as a replacement
	// for an unhealthy machine (a retry).
	//
	// If not set, a retry will happen immediately.
	// +optional
	RetryPeriod metav1.Duration `json:"retryPeriod,omitempty"`

	// minHealthyPeriod defines the duration after which RKE2ControlPlane will consider any failure to a machine unrelated
	// from the previous one. In this case the remediation is not considered a retry anymore, and thus the retry
	// counter restarts from 0. For example, assuming MinHealthyPeriod is set to 1h (default)
	//
	//	M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.
	//	If M1-1 (replacement of M1) has problems within the 1hr after the creation, also
	//	this machine will be remediated and this operation is considered a retry - a problem related
	//	to the original issue happened to M1 -.
	//
	//	If instead the problem on M1-1 is happening after MinHealthyPeriod expired, e.g. four days after
	//	m1-1 has been created as a remediation of M1, the problem on M1-1 is considered unrelated to
	//	the original issue happened to M1.
	//
	// If not set, this value is defaulted to 1h.
	// +optional
	MinHealthyPeriod *metav1.Duration `json:"minHealthyPeriod,omitempty"`
}

// LastRemediationStatus  stores info about last remediation performed.
// NOTE: if for any reason information about last remediation are lost, RetryCount is going to restart from 0 and thus
// more remediations than expected might happen.
type LastRemediationStatus struct {
	// machine is the machine name of the latest machine being remediated.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Machine string `json:"machine"`

	// timestamp is when last remediation happened. It is represented in RFC3339 form and is in UTC.
	// +required
	Timestamp metav1.Time `json:"timestamp"`

	// retryCount used to keep track of remediation retry for the last remediated machine.
	// A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
	// +required
	RetryCount int `json:"retryCount"`
}

// RolloutStrategy describes how to replace existing machines
// with new ones.
type RolloutStrategy struct {
	// Type of rollout. Currently the only supported strategy is "RollingUpdate".
	// Default is RollingUpdate.
	// +optional
	Type RolloutStrategyType `json:"type,omitempty"`

	// Rolling update config params. Present only if RolloutStrategyType = RollingUpdate.
	// +optional
	RollingUpdate *RollingUpdate `json:"rollingUpdate,omitempty"`
}

// RollingUpdate is used to control the desired behavior of rolling update.
type RollingUpdate struct {
	// The maximum number of control planes that can be scheduled above or under the
	// desired number of control planes.
	// Value can be an absolute number 1 or 0.
	// Defaults to 1.
	// Example: when this is set to 1, the control plane can be scaled
	// up immediately when the rolling update starts.
	// +optional
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
}

// RolloutStrategyType defines the rollout strategies for a RKE2ControlPlane.
type RolloutStrategyType string

const (
	// RollingUpdateStrategyType replaces the old control planes by new one using rolling update
	// i.e. gradually scale up or down the old control planes and scale up or down the new one.
	RollingUpdateStrategyType RolloutStrategyType = "RollingUpdate"

	// PreTerminateHookCleanupAnnotation is the annotation RKE2 sets on Machines to ensure it can later remove the
	// etcd member right before Machine termination (i.e. before InfraMachine deletion).
	// For RKE2 we need wait for all other pre-terminate hooks to finish to
	// ensure it runs last (thus ensuring that kubelet is still working while other pre-terminate hooks run
	// as it uses kubelet local mode).
	PreTerminateHookCleanupAnnotation = clusterv1.PreTerminateDeleteHookAnnotationPrefix + "/rke2-cleanup"

	// PreDrainLoadbalancerExclusionAnnotation is the annotation set on Machines to ensure the downstream
	// Node is labeled with `node.kubernetes.io/exclude-from-external-load-balancers`.
	// This allows load balancers as MetalLB to stop advertising this node.
	// The label is added on pre-drain hook to give enough time for the load balancer to react to the change,
	// before the Machine is actually terminated.
	PreDrainLoadbalancerExclusionAnnotation = clusterv1.PreDrainDeleteHookAnnotationPrefix + "/rke2-lb-exclusion"
)

func init() { //nolint:gochecknoinits
	objectTypes = append(objectTypes, &RKE2ControlPlane{}, &RKE2ControlPlaneList{})
}

// GetConditions returns the list of conditions for a RKE2ControlPlane object.
func (r *RKE2ControlPlane) GetConditions() clusterv1.Conditions {
	return r.Status.Conditions
}

// SetConditions sets the list of conditions for a RKE2ControlPlane object.
func (r *RKE2ControlPlane) SetConditions(conditions clusterv1.Conditions) {
	r.Status.Conditions = conditions
}

// GetDesiredVersion returns the desired version of the RKE2ControlPlane using Spec.Version field as a default field.
func (r *RKE2ControlPlane) GetDesiredVersion() string {
	return r.Spec.Version
}
