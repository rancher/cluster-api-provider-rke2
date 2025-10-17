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

package rke2

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher/cluster-api-provider-rke2/pkg/consts"
	bsutil "github.com/rancher/cluster-api-provider-rke2/pkg/util"
)

const (
	// DefaultRKE2ConfigLocation is the default location for the RKE2 config file.
	DefaultRKE2ConfigLocation = "/etc/rancher/rke2/config.yaml"

	// DefaultRKE2CloudProviderConfigLocation is the default location for the RKE2 cloud provider config file.
	DefaultRKE2CloudProviderConfigLocation = "/etc/rancher/rke2/cloud-provider-config"

	// EncryptionConfigurationLocation a location where rke2 looks up for encryption config.
	EncryptionConfigurationLocation = "/var/lib/rancher/rke2/server/cred/encryption-config.json"

	// DefaultRKE2JoinPort is the default port used for joining nodes to the cluster. It is open on the control plane nodes.
	DefaultRKE2JoinPort = 9345

	// CISNodePreparationScript is the script that is used to prepare a node for CIS compliance.
	CISNodePreparationScript = `#!/bin/bash
set -e

# Adding etcd user if not present
if id etcd &>/dev/null; then
    echo 'user etcd already exists'
else
		useradd -r -c "etcd user" -s /sbin/nologin -M etcd -U
fi

YUM_BASED_PARAM_FILE_FOUND=false
TAR_BASED_PARAM_FILE_FOUND=false
INSTALLER_BASED_PARAM_FILE_FOUND=false

# Using RKE2 generated kernel parameters
if [ -f /usr/share/rke2/rke2-cis-sysctl.conf ]; then
		YUM_BASED_PARAM_FILE_FOUND=true
		cp -f /usr/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/90-rke2-cis.conf
fi

if [ -f /usr/local/share/rke2/rke2-cis-sysctl.conf ]; then
		TAR_BASED_PARAM_FILE_FOUND=true
		cp -f /usr/local/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/90-rke2-cis.conf
fi

if [ -f /opt/rke2/share/rke2/rke2-cis-sysctl.conf ]; then
		INSTALLER_BASED_PARAM_FILE_FOUND=true
		cp -f /opt/rke2/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/90-rke2-cis.conf
fi

if [ "$YUM_BASED_PARAM_FILE_FOUND" = false ] && [ "$TAR_BASED_PARAM_FILE_FOUND" = false ] && [ "$INSTALLER_BASED_PARAM_FILE_FOUND" = false ]; then
		echo "No kernel parameters file found"
		exit 1
fi

# Applying kernel parameters
sysctl -p /etc/sysctl.d/90-rke2-cis.conf
`
	//nolint:lll //intentionally compacted to a single line
	encptionConfigTemplate = `{"kind":"EncryptionConfiguration","apiVersion":"apiserver.config.k8s.io/v1","resources":[{"resources":["secrets"],"providers":[{"%s":{"keys":[{"name":"enckey","secret":"%s"}]}},{"identity":{}}]}]}`
)

// ServerConfig is a struct that contains the information needed to generate a RKE2 server config.
type ServerConfig struct {
	AdvertiseAddress                  string   `yaml:"advertise-address,omitempty"`
	AuditPolicyFile                   string   `yaml:"audit-policy-file,omitempty"`
	BindAddress                       string   `yaml:"bind-address,omitempty"`
	CNI                               []string `yaml:"cni,omitempty"`
	CloudControllerManagerExtraEnv    []string `yaml:"cloud-controller-manager-extra-env,omitempty"`
	CloudControllerManagerExtraMounts []string `yaml:"cloud-controller-manager-extra-mount,omitempty"`
	ClusterDNS                        string   `yaml:"cluster-dns,omitempty"`
	ClusterDomain                     string   `yaml:"cluster-domain,omitempty"`
	DisableCloudController            bool     `yaml:"disable-cloud-controller,omitempty"`
	DisableComponents                 []string `yaml:"disable,omitempty"`
	DisableKubeProxy                  bool     `yaml:"disable-kube-proxy,omitempty"`
	DisableScheduler                  bool     `yaml:"disable-scheduler,omitempty"`
	EtcdArgs                          []string `yaml:"etcd-arg,omitempty"`
	EtcdExtraEnv                      []string `yaml:"etcd-extra-env,omitempty"`
	EtcdExtraMounts                   []string `yaml:"etcd-extra-mount,omitempty"`
	EtcdImage                         string   `yaml:"etcd-image,omitempty"`
	EtcdDisableSnapshots              *bool    `yaml:"etcd-disable-snapshots,omitempty"`
	EtcdExposeMetrics                 bool     `yaml:"etcd-expose-metrics,omitempty"`
	EtcdS3                            bool     `yaml:"etcd-s3,omitempty"`
	EtcdS3AccessKey                   string   `yaml:"etcd-s3-access-key,omitempty"`
	EtcdS3Bucket                      string   `yaml:"etcd-s3-bucket,omitempty"`
	EtcdS3Endpoint                    string   `yaml:"etcd-s3-endpoint,omitempty"`
	EtcdS3EndpointCA                  string   `yaml:"etcd-s3-endpoint-ca,omitempty"`
	EtcdS3Folder                      string   `yaml:"etcd-s3-folder,omitempty"`
	EtcdS3Region                      string   `yaml:"etcd-s3-region,omitempty"`
	EtcdS3SecretKey                   string   `yaml:"etcd-s3-secret-key,omitempty"`
	EtcdS3SkipSslVerify               bool     `yaml:"etcd-s3-skip-ssl-verify,omitempty"`
	EtcdSnapshotDir                   string   `yaml:"etcd-snapshot-dir,omitempty"`
	EtcdSnapshotName                  string   `yaml:"etcd-snapshot-name,omitempty"`
	EtcdSnapshotRetention             string   `yaml:"etcd-snapshot-retention,omitempty"`
	EtcdSnapshotScheduleCron          string   `yaml:"etcd-snapshot-schedule-cron,omitempty"`
	KubeAPIServerArgs                 []string `yaml:"kube-apiserver-arg,omitempty"`
	KubeAPIserverExtraEnv             []string `yaml:"kube-apiserver-extra-env,omitempty"`
	KubeAPIserverExtraMounts          []string `yaml:"kube-apiserver-extra-mount,omitempty"`
	KubeAPIserverImage                string   `yaml:"kube-apiserver-image,omitempty"`
	KubeControllerManagerArgs         []string `yaml:"kube-controller-manager-arg,omitempty"`
	KubeControllerManagerExtraEnv     []string `yaml:"kube-controller-manager-extra-env,omitempty"`
	KubeControllerManagerExtraMounts  []string `yaml:"kube-controller-manager-extra-mount,omitempty"`
	KubeControllerManagerImage        string   `yaml:"kube-controller-manager-image,omitempty"`
	KubeSchedulerArgs                 []string `yaml:"kube-scheduler-arg,omitempty"`
	KubeSchedulerExtraEnv             []string `yaml:"kube-scheduler-extra-env,omitempty"`
	KubeSchedulerExtraMounts          []string `yaml:"kube-scheduler-extra-mount,omitempty"`
	KubeSchedulerImage                string   `yaml:"kube-scheduler-image,omitempty"`
	ServiceNodePortRange              string   `yaml:"service-node-port-range,omitempty"`
	TLSSan                            []string `yaml:"tls-san,omitempty"`
	EmbeddedRegistry                  bool     `yaml:"embedded-registry,omitempty"`
	DatastoreEndpoint                 string   `yaml:"datastore-endpoint,omitempty"`
	DatastoreCAFile                   string   `yaml:"datastore-cafile,omitempty"`
	DatastoreCertFile                 string   `yaml:"datastore-certfile,omitempty"`
	DatastoreKeyFile                  string   `yaml:"datastore-keyfile,omitempty"`
	SecretsEncryptionProvider         string   `yaml:"secrets-encryption-provider"`

	// We don't expose these fields in the API
	ClusterCIDR string `yaml:"cluster-cidr,omitempty"`
	ServiceCIDR string `yaml:"service-cidr,omitempty"`

	// Fields below are missing from our API and RKE2 docs
	AirgapExtraRegistry       string `yaml:"airgap-extra-registry,omitempty"`
	DisableAPIserver          bool   `yaml:"disable-apiserver,omitempty"`
	DisableControllerManager  bool   `yaml:"disable-controller-manager,omitempty"`
	EgressSelectorMode        string `yaml:"egress-selector-mode,omitempty"`
	EnablePprof               bool   `yaml:"enable-pprof,omitempty"`
	EnableServiceLoadBalancer bool   `yaml:"enable-servicelb,omitempty"`
	EtcdS3Insecure            bool   `yaml:"etcd-s3-insecure,omitempty"`
	EtcdS3Timeout             string `yaml:"etcd-s3-timeout,omitempty"`
	EtcdSnapshotCompress      string `yaml:"etcd-snapshot-compress,omitempty"`
	ServicelbNamespace        string `yaml:"servicelb-namespace,omitempty"`

	rke2AgentConfig `yaml:",inline"`
}

// ServerConfigOpts is a struct that contains the information needed to generate a RKE2 server config.
type ServerConfigOpts struct {
	Cluster              clusterv1.Cluster
	ControlPlaneEndpoint string
	Token                string
	ServerURL            string
	ServerConfig         controlplanev1.RKE2ServerConfig
	AgentConfig          bootstrapv1.RKE2AgentConfig
	Ctx                  context.Context
	Client               client.Client
	Version              string
}

func newRKE2ServerConfig(opts ServerConfigOpts) (*ServerConfig, []bootstrapv1.File, error) { // nolint:gocyclo
	rke2ServerConfig := &ServerConfig{}
	files := []bootstrapv1.File{}
	rke2ServerConfig.AdvertiseAddress = opts.ServerConfig.AdvertiseAddress

	if opts.ServerConfig.AuditPolicySecret != nil {
		auditPolicySecret := &corev1.Secret{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.ServerConfig.AuditPolicySecret.Name,
			Namespace: opts.ServerConfig.AuditPolicySecret.Namespace,
		}, auditPolicySecret); err != nil {
			return nil, nil, fmt.Errorf("failed to get audit policy secret: %w", err)
		}

		auditPolicy, ok := auditPolicySecret.Data["audit-policy.yaml"]
		if !ok {
			return nil, nil, errors.New("audit policy secret is missing audit-policy.yaml key")
		}

		rke2ServerConfig.AuditPolicyFile = "/etc/rancher/rke2/audit-policy.yaml"

		files = append(files, bootstrapv1.File{
			Path:        rke2ServerConfig.AuditPolicyFile,
			Content:     string(auditPolicy),
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.DefaultFileMode,
		})
	}

	if opts.Cluster.Spec.ClusterNetwork != nil &&
		opts.Cluster.Spec.ClusterNetwork.Pods != nil &&
		len(opts.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks) > 0 {
		rke2ServerConfig.ClusterCIDR = strings.Join(opts.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks, ",")
	}

	if opts.Cluster.Spec.ClusterNetwork != nil &&
		opts.Cluster.Spec.ClusterNetwork.Services != nil &&
		len(opts.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks) > 0 {
		rke2ServerConfig.ServiceCIDR = strings.Join(opts.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks, ",")
	}

	rke2ServerConfig.BindAddress = opts.ServerConfig.BindAddress
	if opts.ServerConfig.CNIMultusEnable {
		rke2ServerConfig.CNI = append([]string{"multus"}, string(opts.ServerConfig.CNI))
	} else if opts.ServerConfig.CNI != "" {
		rke2ServerConfig.CNI = []string{string(opts.ServerConfig.CNI)}
	}

	rke2ServerConfig.ClusterDNS = opts.ServerConfig.ClusterDNS
	rke2ServerConfig.ClusterDomain = opts.ServerConfig.ClusterDomain

	if opts.ServerConfig.CloudProviderConfigMap != nil {
		cloudProviderConfigMap := &corev1.ConfigMap{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.ServerConfig.CloudProviderConfigMap.Name,
			Namespace: opts.ServerConfig.CloudProviderConfigMap.Namespace,
		}, cloudProviderConfigMap); err != nil {
			return nil, nil, fmt.Errorf("failed to get cloud provider config map: %w", err)
		}

		cloudProviderConfig, ok := cloudProviderConfigMap.Data["cloud-config"]
		if !ok {
			return nil, nil, errors.New("cloud provider config map is missing cloud-config key")
		}

		rke2ServerConfig.CloudProviderConfig = "/etc/rancher/rke2/cloud-provider-config"

		files = append(files, bootstrapv1.File{
			Path:        rke2ServerConfig.CloudProviderConfig,
			Content:     cloudProviderConfig,
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.DefaultFileMode,
		})
	}

	rke2ServerConfig.CloudProviderName = opts.ServerConfig.CloudProviderName
	rke2ServerConfig.DisableComponents = func() []string {
		disabled := []string{}
		for _, plugin := range opts.ServerConfig.DisableComponents.PluginComponents {
			disabled = append(disabled, string(plugin))
		}

		return disabled
	}()

	for _, component := range opts.ServerConfig.DisableComponents.KubernetesComponents {
		switch component {
		case controlplanev1.KubeProxy:
			rke2ServerConfig.DisableKubeProxy = true
		case controlplanev1.Scheduler:
			rke2ServerConfig.DisableScheduler = true
		case controlplanev1.CloudController:
			rke2ServerConfig.DisableCloudController = true
		}
	}

	rke2ServerConfig.EtcdDisableSnapshots = opts.ServerConfig.Etcd.BackupConfig.DisableAutomaticSnapshots
	rke2ServerConfig.EtcdExposeMetrics = opts.ServerConfig.Etcd.ExposeMetrics

	if opts.ServerConfig.Etcd.BackupConfig.S3 != nil {
		rke2ServerConfig.EtcdS3 = true
		awsCredentialsSecret := &corev1.Secret{}
		accessKeyID, secretAccessKey := []byte{}, []byte{}

		if opts.ServerConfig.Etcd.BackupConfig.S3.S3CredentialSecret != nil {
			if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
				Name:      opts.ServerConfig.Etcd.BackupConfig.S3.S3CredentialSecret.Name,
				Namespace: opts.ServerConfig.Etcd.BackupConfig.S3.S3CredentialSecret.Namespace,
			}, awsCredentialsSecret); err != nil {
				return nil, nil, fmt.Errorf("failed to get aws credentials secret: %w", err)
			}

			var ok bool
			accessKeyID, ok = awsCredentialsSecret.Data["aws_access_key_id"]

			if !ok {
				return nil, nil, errors.New("aws credentials secret is missing aws_access_key_id")
			}

			secretAccessKey, ok = awsCredentialsSecret.Data["aws_secret_access_key"]

			if !ok {
				return nil, nil, errors.New("aws credentials secret is missing aws_secret_access_key")
			}
		}

		rke2ServerConfig.EtcdS3AccessKey = string(accessKeyID)
		rke2ServerConfig.EtcdS3SecretKey = string(secretAccessKey)
		rke2ServerConfig.EtcdS3Bucket = opts.ServerConfig.Etcd.BackupConfig.S3.Bucket
		rke2ServerConfig.EtcdS3Region = opts.ServerConfig.Etcd.BackupConfig.S3.Region
		rke2ServerConfig.EtcdS3Folder = opts.ServerConfig.Etcd.BackupConfig.S3.Folder
		rke2ServerConfig.EtcdS3Endpoint = opts.ServerConfig.Etcd.BackupConfig.S3.Endpoint

		if opts.ServerConfig.Etcd.BackupConfig.S3.EndpointCASecret != nil {
			endpointCAsecret := &corev1.Secret{}
			if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
				Name:      opts.ServerConfig.Etcd.BackupConfig.S3.EndpointCASecret.Name,
				Namespace: opts.ServerConfig.Etcd.BackupConfig.S3.EndpointCASecret.Namespace,
			}, endpointCAsecret); err != nil {
				return nil, nil, fmt.Errorf("failed to get aws credentials secret: %w", err)
			}

			caCert, ok := endpointCAsecret.Data["ca.pem"]
			if !ok {
				return nil, nil, errors.New("endpoint CA secret is missing ca.pem")
			}

			rke2ServerConfig.EtcdS3EndpointCA = "/etc/rancher/rke2/etcd-s3-ca.crt"

			files = append(files, bootstrapv1.File{
				Path:        rke2ServerConfig.EtcdS3EndpointCA,
				Content:     string(caCert),
				Owner:       consts.DefaultFileOwner,
				Permissions: "0640",
			})
		}

		rke2ServerConfig.EtcdS3SkipSslVerify = !opts.ServerConfig.Etcd.BackupConfig.S3.EnforceSSLVerify
	}

	rke2ServerConfig.EtcdSnapshotDir = opts.ServerConfig.Etcd.BackupConfig.Directory
	rke2ServerConfig.EtcdSnapshotName = opts.ServerConfig.Etcd.BackupConfig.SnapshotName
	rke2ServerConfig.EtcdSnapshotRetention = opts.ServerConfig.Etcd.BackupConfig.Retention
	rke2ServerConfig.EtcdSnapshotScheduleCron = opts.ServerConfig.Etcd.BackupConfig.ScheduleCron

	if opts.ServerConfig.Etcd.CustomConfig != nil {
		rke2ServerConfig.EtcdArgs = opts.ServerConfig.Etcd.CustomConfig.ExtraArgs
		rke2ServerConfig.EtcdImage = opts.ServerConfig.Etcd.CustomConfig.OverrideImage
		rke2ServerConfig.EtcdExtraMounts = componentMapToSlice(extraMount, opts.ServerConfig.Etcd.CustomConfig.ExtraMounts)
		rke2ServerConfig.EtcdExtraEnv = componentMapToSlice(extraEnv, opts.ServerConfig.Etcd.CustomConfig.ExtraEnv)
	}

	rke2ServerConfig.ServiceNodePortRange = opts.ServerConfig.ServiceNodePortRange
	rke2ServerConfig.TLSSan = append(opts.ServerConfig.TLSSan, opts.ControlPlaneEndpoint)

	if opts.ServerConfig.KubeAPIServer != nil {
		rke2ServerConfig.KubeAPIServerArgs = opts.ServerConfig.KubeAPIServer.ExtraArgs
		rke2ServerConfig.KubeAPIserverImage = opts.ServerConfig.KubeAPIServer.OverrideImage
		rke2ServerConfig.KubeAPIserverExtraMounts = componentMapToSlice(extraMount, opts.ServerConfig.KubeAPIServer.ExtraMounts)
		rke2ServerConfig.KubeAPIserverExtraEnv = componentMapToSlice(extraEnv, opts.ServerConfig.KubeAPIServer.ExtraEnv)
	}

	if opts.ServerConfig.KubeScheduler != nil {
		rke2ServerConfig.KubeSchedulerArgs = opts.ServerConfig.KubeScheduler.ExtraArgs
		rke2ServerConfig.KubeSchedulerImage = opts.ServerConfig.KubeScheduler.OverrideImage
		rke2ServerConfig.KubeSchedulerExtraMounts = componentMapToSlice(extraMount, opts.ServerConfig.KubeScheduler.ExtraMounts)
		rke2ServerConfig.KubeSchedulerExtraEnv = componentMapToSlice(extraEnv, opts.ServerConfig.KubeScheduler.ExtraEnv)
	}

	if opts.ServerConfig.SecretsEncryptionProvider != nil {
		rke2ServerConfig.SecretsEncryptionProvider = opts.ServerConfig.SecretsEncryptionProvider.Provider
		encryptionSecret := &corev1.Secret{}

		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.ServerConfig.SecretsEncryptionProvider.EncryptionKeySecret.Name,
			Namespace: opts.ServerConfig.SecretsEncryptionProvider.EncryptionKeySecret.Namespace,
		}, encryptionSecret); err != nil {
			return nil, nil, fmt.Errorf("failed to get encryptionKey secret: %w", err)
		}

		key, ok := encryptionSecret.Data["encryptionKey"]
		if !ok {
			return nil, nil, errors.New("encryptionKey secret missing 'encryptionKey' key")
		}

		files = append(files, bootstrapv1.File{
			Path:        EncryptionConfigurationLocation,
			Content:     generateEncryptionConfig(opts.ServerConfig.SecretsEncryptionProvider.Provider, string(key)),
			Owner:       consts.DefaultFileOwner,
			Permissions: "0400",
		})
	}

	if opts.ServerConfig.KubeControllerManager != nil {
		rke2ServerConfig.KubeControllerManagerArgs = opts.ServerConfig.KubeControllerManager.ExtraArgs
		rke2ServerConfig.KubeControllerManagerImage = opts.ServerConfig.KubeControllerManager.OverrideImage
		rke2ServerConfig.KubeControllerManagerExtraMounts = componentMapToSlice(extraMount, opts.ServerConfig.KubeControllerManager.ExtraMounts)
		rke2ServerConfig.KubeControllerManagerExtraEnv = componentMapToSlice(extraEnv, opts.ServerConfig.KubeControllerManager.ExtraEnv)
	}

	if opts.ServerConfig.CloudControllerManager != nil {
		rke2ServerConfig.CloudControllerManagerExtraMounts = componentMapToSlice(extraMount, opts.ServerConfig.CloudControllerManager.ExtraMounts)
		rke2ServerConfig.CloudControllerManagerExtraEnv = componentMapToSlice(extraEnv, opts.ServerConfig.CloudControllerManager.ExtraEnv)
	}

	rke2ServerConfig.EmbeddedRegistry = opts.ServerConfig.EmbeddedRegistry

	if opts.ServerConfig.ExternalDatastoreSecret != nil {
		externalDatastoreSecret := &corev1.Secret{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.ServerConfig.ExternalDatastoreSecret.Name,
			Namespace: opts.ServerConfig.ExternalDatastoreSecret.Namespace,
		}, externalDatastoreSecret); err != nil {
			return nil, nil, fmt.Errorf("failed to get external datastore secret: %w", err)
		}

		endpoint, ok := externalDatastoreSecret.Data["endpoint"]
		if !ok {
			return nil, nil, errors.New("external datastore secret is missing endpoint key")
		}

		rke2ServerConfig.DatastoreEndpoint = string(endpoint)

		// Setting a CA file for the datastore is optional
		caCert, ok := externalDatastoreSecret.Data["ca.pem"]
		if ok {
			rke2ServerConfig.DatastoreCAFile = "/etc/rancher/rke2/datastore-ca.crt"

			files = append(files, bootstrapv1.File{
				Path:        rke2ServerConfig.DatastoreCAFile,
				Content:     string(caCert),
				Owner:       consts.DefaultFileOwner,
				Permissions: "0640",
			})
		}

		// For datastores that support client certificated based authentication, `cert.pem` is expected
		// to hold the certificate and `key.pem` is expected to hold the certificate's private key.
		// Setting them is optional but if `cert.pem` is set then `key.pem` must also be set.
		// https://docs.rke2.io/datastore/external#external-datastore-configuration-parameters
		cert, ok := externalDatastoreSecret.Data["cert.pem"]
		if ok {
			// Ensure that the key is also present.
			key, ok := externalDatastoreSecret.Data["key.pem"]
			if !ok {
				return nil, nil, errors.New("external datastore secret is missing key.pem key")
			}

			rke2ServerConfig.DatastoreCertFile = "/etc/rancher/rke2/datastore-cert.crt"

			files = append(files, bootstrapv1.File{
				Path:        rke2ServerConfig.DatastoreCertFile,
				Content:     string(cert),
				Owner:       consts.DefaultFileOwner,
				Permissions: "0640",
			})

			rke2ServerConfig.DatastoreKeyFile = "/etc/rancher/rke2/datastore-key.crt"

			files = append(files, bootstrapv1.File{
				Path:        rke2ServerConfig.DatastoreKeyFile,
				Content:     string(key),
				Owner:       consts.DefaultFileOwner,
				Permissions: "0640",
			})
		}
	}

	return rke2ServerConfig, files, nil
}

type rke2AgentConfig struct {
	ContainerRuntimeEndpoint       string   `yaml:"container-runtime-endpoint,omitempty"`
	CloudProviderConfig            string   `yaml:"cloud-provider-config,omitempty"`
	CloudProviderName              string   `yaml:"cloud-provider-name,omitempty"`
	DataDir                        string   `yaml:"data-dir,omitempty"`
	ImageCredentialProviderConfig  string   `yaml:"image-credential-provider-config,omitempty"`
	ImageCredentialProviderBinDir  string   `yaml:"image-credential-provider-bin-dir,omitempty"`
	KubeProxyArgs                  []string `yaml:"kube-proxy-arg,omitempty"`
	KubeProxyExtraEnv              []string `yaml:"kube-proxy-extra-env,omitempty"`
	KubeProxyExtraMounts           []string `yaml:"kube-proxy-extra-mount,omitempty"`
	KubeProxyImage                 string   `yaml:"kube-proxy-image,omitempty"`
	KubeletArgs                    []string `yaml:"kubelet-arg,omitempty"`
	KubeletPath                    string   `yaml:"kubelet-path,omitempty"`
	LbServerPort                   int      `yaml:"lb-server-port,omitempty"`
	NodeLabels                     []string `yaml:"node-label,omitempty"`
	NodeTaints                     []string `yaml:"node-taint,omitempty"`
	Profile                        string   `yaml:"profile,omitempty"`
	ProtectKernelDefaults          bool     `yaml:"protect-kernel-defaults,omitempty"`
	PodSecurityAdmissionConfigFile string   `yaml:"pod-security-admission-config-file,omitempty"` // new flag, not present in the RKE2 docs yet
	ResolvConf                     string   `yaml:"resolv-conf,omitempty"`
	RuntimeImage                   string   `yaml:"runtime-image,omitempty"`
	Selinux                        bool     `yaml:"selinux,omitempty"`
	Server                         string   `yaml:"server,omitempty"`
	Snapshotter                    string   `yaml:"snapshotter,omitempty"`
	Token                          string   `yaml:"token,omitempty"`

	// We don't expose these in the API
	PauseImage      string `yaml:"pause-image,omitempty"`
	PrivateRegistry string `yaml:"private-registry,omitempty"`

	SystemDefaultRegistry string `yaml:"system-default-registry,omitempty"`

	NodeExternalIp string `yaml:"node-external-ip,omitempty"`
	NodeIp         string `yaml:"node-ip,omitempty"`
	NodeName       string `yaml:"node-name,omitempty"`
}

// AgentConfigOpts is a struct that holds the information needed to generate the rke2 server config.
type AgentConfigOpts struct {
	ServerURL              string
	Token                  string
	AgentConfig            bootstrapv1.RKE2AgentConfig
	Ctx                    context.Context
	Client                 client.Client
	CloudProviderName      string
	CloudProviderConfigMap *corev1.ObjectReference
	Version                string
}

func newRKE2AgentConfig(opts AgentConfigOpts) (*rke2AgentConfig, []bootstrapv1.File, error) {
	rke2AgentConfig := &rke2AgentConfig{}
	files := []bootstrapv1.File{}
	rke2AgentConfig.ContainerRuntimeEndpoint = opts.AgentConfig.ContainerRuntimeEndpoint

	if opts.AgentConfig.CISProfile != "" {
		if !bsutil.ProfileCompliant(opts.AgentConfig.CISProfile, opts.Version) {
			return nil, nil, fmt.Errorf("profile %q is not supported for version %q", opts.AgentConfig.CISProfile, opts.Version)
		}

		files = append(files, bootstrapv1.File{
			Path:        "/opt/rke2-cis-script.sh",
			Content:     CISNodePreparationScript,
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.FileModeRootExecutable,
		})
		rke2AgentConfig.Profile = string(opts.AgentConfig.CISProfile)
	}

	if opts.CloudProviderConfigMap != nil {
		cloudProviderConfigMap := &corev1.ConfigMap{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.CloudProviderConfigMap.Name,
			Namespace: opts.CloudProviderConfigMap.Namespace,
		}, cloudProviderConfigMap); err != nil {
			return nil, nil, fmt.Errorf("failed to get cloud provider config map: %w", err)
		}

		cloudProviderConfig, ok := cloudProviderConfigMap.Data["cloud-config"]

		if !ok {
			return nil, nil, errors.New("cloud provider config map is missing cloud-config key")
		}

		rke2AgentConfig.CloudProviderConfig = DefaultRKE2CloudProviderConfigLocation

		files = append(files, bootstrapv1.File{
			Path:        rke2AgentConfig.CloudProviderConfig,
			Content:     cloudProviderConfig,
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.DefaultFileMode,
		})
	}

	rke2AgentConfig.CloudProviderName = opts.CloudProviderName
	rke2AgentConfig.DataDir = opts.AgentConfig.DataDir

	if opts.AgentConfig.ImageCredentialProviderConfigMap != nil {
		imageCredentialProviderCM := &corev1.ConfigMap{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.AgentConfig.ImageCredentialProviderConfigMap.Name,
			Namespace: opts.AgentConfig.ImageCredentialProviderConfigMap.Namespace,
		}, imageCredentialProviderCM); err != nil {
			return nil, nil, fmt.Errorf("failed to get image credential provider config map: %w", err)
		}

		credentialConfig, ok := imageCredentialProviderCM.Data["credential-config.yaml"]

		if !ok {
			return nil, nil, errors.New("image credential provider config map is missing config.yaml")
		}

		credentialProviderBinaries, ok := imageCredentialProviderCM.Data["credential-provider-binaries"]
		if !ok {
			return nil, nil, errors.New("image credential provider config map is missing credential-provider-binaries")
		}

		rke2AgentConfig.ImageCredentialProviderBinDir = credentialProviderBinaries
		rke2AgentConfig.ImageCredentialProviderConfig = "/etc/rancher/rke2/credential-config.yaml"

		files = append(files, bootstrapv1.File{
			Path:        rke2AgentConfig.ImageCredentialProviderConfig,
			Content:     credentialConfig,
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.DefaultFileMode,
		})
	}

	rke2AgentConfig.KubeletPath = opts.AgentConfig.KubeletPath
	if opts.AgentConfig.Kubelet != nil {
		rke2AgentConfig.KubeletArgs = opts.AgentConfig.Kubelet.ExtraArgs
	}

	rke2AgentConfig.LbServerPort = opts.AgentConfig.LoadBalancerPort
	rke2AgentConfig.NodeLabels = opts.AgentConfig.NodeLabels
	rke2AgentConfig.NodeTaints = opts.AgentConfig.NodeTaints
	rke2AgentConfig.ProtectKernelDefaults = opts.AgentConfig.ProtectKernelDefaults
	rke2AgentConfig.PodSecurityAdmissionConfigFile = opts.AgentConfig.PodSecurityAdmissionConfigFile

	if opts.AgentConfig.ResolvConf != nil {
		resolvConfCM := &corev1.ConfigMap{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.AgentConfig.ResolvConf.Name,
			Namespace: opts.AgentConfig.ResolvConf.Namespace,
		}, resolvConfCM); err != nil {
			return nil, nil, fmt.Errorf("failed to get resolv.conf config map: %w", err)
		}

		resolvConf, ok := resolvConfCM.Data["resolv.conf"]
		if !ok {
			return nil, nil, errors.New("resolv conf config map is missing resolv.conf")
		}

		rke2AgentConfig.ResolvConf = "/etc/rancher/rke2/resolv.conf"

		files = append(files, bootstrapv1.File{
			Path:        rke2AgentConfig.ResolvConf,
			Content:     resolvConf,
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.DefaultFileMode,
		})
	}

	rke2AgentConfig.RuntimeImage = opts.AgentConfig.RuntimeImage
	rke2AgentConfig.Selinux = opts.AgentConfig.EnableContainerdSElinux
	rke2AgentConfig.Server = opts.ServerURL
	rke2AgentConfig.Snapshotter = opts.AgentConfig.Snapshotter

	if opts.AgentConfig.KubeProxy != nil {
		rke2AgentConfig.KubeProxyArgs = opts.AgentConfig.KubeProxy.ExtraArgs
		rke2AgentConfig.KubeProxyImage = opts.AgentConfig.KubeProxy.OverrideImage
		rke2AgentConfig.KubeProxyExtraMounts = componentMapToSlice(extraMount, opts.AgentConfig.KubeProxy.ExtraMounts)
		rke2AgentConfig.KubeProxyExtraEnv = componentMapToSlice(extraEnv, opts.AgentConfig.KubeProxy.ExtraEnv)
	}

	rke2AgentConfig.Token = opts.Token
	rke2AgentConfig.SystemDefaultRegistry = opts.AgentConfig.SystemDefaultRegistry

	return rke2AgentConfig, files, nil
}

// GenerateInitControlPlaneConfig generates the rke2 server and agent config for the init control plane node.
func GenerateInitControlPlaneConfig(opts ServerConfigOpts) (*ServerConfig, []bootstrapv1.File, error) {
	if opts.Token == "" {
		return nil, nil, errors.New("token is required")
	}

	rke2ServerConfig, serverFiles, err := newRKE2ServerConfig(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rke2 server config: %w", err)
	}

	rke2AgentConfig, agentFiles, err := newRKE2AgentConfig(AgentConfigOpts{
		AgentConfig:            opts.AgentConfig,
		Client:                 opts.Client,
		Ctx:                    opts.Ctx,
		Token:                  opts.Token,
		Version:                opts.Version,
		CloudProviderConfigMap: opts.ServerConfig.CloudProviderConfigMap,
		CloudProviderName:      opts.ServerConfig.CloudProviderName,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rke2 agent config: %w", err)
	}

	rke2ServerConfig.rke2AgentConfig = *rke2AgentConfig

	return rke2ServerConfig, append(serverFiles, agentFiles...), nil
}

// GenerateJoinControlPlaneConfig generates the rke2 agent config for joining a control plane node.
func GenerateJoinControlPlaneConfig(opts ServerConfigOpts) (*ServerConfig, []bootstrapv1.File, error) {
	if opts.ServerURL == "" {
		return nil, nil, errors.New("server url is required")
	}

	if opts.Token == "" {
		return nil, nil, errors.New("token is required")
	}

	rke2ServerConfig, serverFiles, err := newRKE2ServerConfig(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rke2 server config: %w", err)
	}

	rke2AgentConfig, agentFiles, err := newRKE2AgentConfig(AgentConfigOpts{
		AgentConfig:            opts.AgentConfig,
		Client:                 opts.Client,
		Ctx:                    opts.Ctx,
		ServerURL:              opts.ServerURL,
		Token:                  opts.Token,
		Version:                opts.Version,
		CloudProviderConfigMap: opts.ServerConfig.CloudProviderConfigMap,
		CloudProviderName:      opts.ServerConfig.CloudProviderName,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rke2 agent config: %w", err)
	}

	rke2ServerConfig.rke2AgentConfig = *rke2AgentConfig

	return rke2ServerConfig, append(serverFiles, agentFiles...), nil
}

// GenerateWorkerConfig generates the rke2 agent config and files.
func GenerateWorkerConfig(opts AgentConfigOpts) (*rke2AgentConfig, []bootstrapv1.File, error) {
	if opts.ServerURL == "" {
		return nil, nil, errors.New("server url is required")
	}

	if opts.Token == "" {
		return nil, nil, errors.New("token is required")
	}

	rke2AgentConfig, agentFiles, err := newRKE2AgentConfig(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rke2 agent config: %w", err)
	}

	return rke2AgentConfig, agentFiles, nil
}

type componentType string

const (
	extraEnv   componentType = "env"
	extraMount componentType = "mount"
)

func componentMapToSlice(componentType componentType, input map[string]string) []string {
	separator := ""

	switch componentType {
	case extraEnv:
		separator = "="
	case extraMount:
		separator = ":"
	}

	result := []string{}

	for key, value := range input {
		if key == "" || (key == "" && value == "") {
			continue
		}

		result = append(result, fmt.Sprintf("%s%s%s", key, separator, value))
	}

	return result
}

func generateEncryptionConfig(provider, key string) string {
	return fmt.Sprintf(encptionConfigTemplate, provider, key)
}
