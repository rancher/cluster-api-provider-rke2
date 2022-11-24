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
	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha1"

	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha1"
)

const DefaultRKE2ConfigLocation = "/etc/rancher/rke2/config.yaml"
const DefaultRKE2JoinPort = 9345

type RKE2ServerConfig struct {
	// TODO: check what this is supposed to do and modify the struct accordingly
	TLSSan                            []string `json:"tls-san,omitempty"`
	BindAddress                       string   `json:"bind-address,omitempty"`
	AdvertiseAddress                  string   `json:"advertise-address,omitempty"`
	ClusterCidr                       string   `json:"cluster-cidr,omitempty"`
	ServiceCidr                       string   `json:"service-cidr,omitempty"`
	ClusterDNS                        string   `json:"cluster-dns,omitempty"`
	ClusterDomain                     string   `json:"cluster-domain,omitempty"`
	DisableComponents                 []string `json:"disable,omitempty"`
	CNI                               string   `json:"cni"`
	EnableServiceLoadBalancer         bool     `json:"enable-servicelb,omitempty"`
	DisableScheduler                  bool     `json:"disable-scheduler,omitempty"`
	DisableCloudController            bool     `json:"disable-cloud-controller,omitempty"`
	DisableKubeProxy                  bool     `json:"disable-kube-proxy,omitempty"`
	DisableAPIserver                  bool     `json:"disable-apiserver,omitempty"`
	DisableControllerManager          bool     `json:"disable-controller-manager,omitempty"`
	DisableEtcd                       bool     `json:"disable-etcd,omitempty"`
	EtcdDisableSnapshots              bool     `json:"etcd-disable-snapshots,omitempty"`
	EtcdSnapshotScheduleCron          string   `json:"etcd-snapshot-schedule-cron,omitempty"`
	EtcdSnapshotRetention             string   `json:"etcd-snapshot-retention,omitempty"`
	EtcdSnapshotDir                   string   `json:"etcd-snapshot-dir,omitempty"`
	EtcdSnapshotName                  string   `json:"etcd-snapshot-name,omitempty"`
	EtcdSnapshotCompress              string   `json:"etcd-snapshot-compress,omitempty"`
	EgressSelectorMode                string   `json:"egress-selector-mode,omitempty"`
	AgentToken                        string   `json:"agent-token,omitempty"`
	AgentTokenFile                    string   `json:"agent-token-file,omitempty"`
	Server                            string   `json:"server,omitempty"`
	Snapshotter                       string   `json:"snapshotter,omitempty"`
	ServiceNodePortRange              string   `json:"service-node-port-range,omitempty"`
	EtcdExposeMetrics                 bool     `json:"etcd-expose-metrics,omitempty"`
	AirgapExtraRegistry               string   `json:"airgap-extra-registry,omitempty"`
	EtcdS3                            bool     `json:"etcd-s3,omitempty"`
	EtcdS3Endpoint                    string   `json:"etcd-s3-endpoint,omitempty"`
	EtcdS3EndpointCa                  string   `json:"etcd-s3-endpoint-ca,omitempty"`
	EtcdS3SkipSslVerify               bool     `json:"etcd-s3-skip-ssl-verify,omitempty"`
	EtcdS3AccessKey                   string   `json:"etcd-s3-access-key,omitempty"`
	EtcdS3SecretKey                   string   `json:"etcd-s3-secret-key,omitempty"`
	EtcdS3Bucket                      string   `json:"etcd-s3-bucket,omitempty"`
	EtcdS3Region                      string   `json:"etcd-s3-region,omitempty"`
	EtcdS3Folder                      string   `json:"etcd-s3-folder,omitempty"`
	EtcdS3Insecure                    bool     `json:"etcd-s3-insecure,omitempty"`
	EtcdS3Timeout                     string   `json:"etcd-s3-timeout,omitempty"`
	EnablePprof                       bool     `json:"enable-pprof,omitempty"`
	ServicelbNamespace                string   `json:"servicelb-namespace,omitempty"`
	KubeAPIServerArgs                 []string `json:"kube-apiserver-arg,omitempty"`
	KubeAPIserverImage                string   `json:"kube-apiserver-image,omitempty"`
	KubeAPIserverExtraMounts          []string `json:"kube-apiserver-extra-mount,omitempty"`
	KubeAPIserverExtraEnvs            []string `json:"kube-apiserver-extra-env,omitempty"`
	EtcdArgs                          []string `json:"etcd-arg,omitempty"`
	KubeSchedulerArgs                 []string `json:"kube-scheduler-arg,omitempty"`
	KubeSchedulerImage                string   `json:"kube-scheduler-image,omitempty"`
	KubeSchedulerExtraMounts          []string `json:"kube-scheduler-extra-mount,omitempty"`
	KubeSchedulerExtraEnvs            []string `json:"kube-scheduler-extra-env,omitempty"`
	KubeControllerManagerArgs         []string `json:"kube-controller-manager-arg,omitempty"`
	KubeControllerManagerImage        string   `json:"kube-controller-manager-image,omitempty"`
	KubeControllerManagerExtraMounts  []string `json:"kube-controller-manager-extra-mount,omitempty"`
	KubeControllerManagerExtraEnvs    []string `json:"kube-controller-manager-extra-env,omitempty"`
	EtcdExtraMounts                   []string `json:"etcd-extra-mount,omitempty"`
	EtcdExtraEnvs                     []string `json:"etcd-extra-env,omitempty"`
	CloudControllerManagerExtraMounts []string `json:"cloud-controller-manager-extra-mount,omitempty"`
	CloudControllerManagerExtraEnvs   []string `json:"cloud-controller-manager-extra-env,omitempty"`
	RKE2AgentConfig                   `json:",inline"`
}

type RKE2AgentConfig struct {
	// TODO: check what this is supposed to do and modify the struct accordingly
	Token                          string   `json:"token,omitempty"`
	Server                         string   `json:"server,omitempty"`
	DataDir                        string   `json:"data-dir"`
	NodeName                       string   `json:"node-name,omitempty"`
	NodeLabels                     []string `json:"node-label,omitempty"`
	NodeTaints                     []string `json:"node-taint,omitempty"`
	ImageCredentialProviderBinDir  string   `json:"image-credential-provider-bin-dir,omitempty"`
	ImageCredentialProviderConfig  string   `json:"image-credential-provider-config,omitempty"`
	ContainerRuntimeEndpoint       string   `json:"container-runtime-endpoint,omitempty"`
	PrivateRegistry                string   `json:"private-registry,omitempty"`
	NodeIp                         string   `json:"node-ip,omitempty"`
	NodeExternalIp                 string   `json:"node-external-ip,omitempty"`
	ResolvConf                     string   `json:"resolv-conf,omitempty"`
	KubeletArgs                    []string `json:"kubelet-arg,omitempty"`
	KubeProxyArgs                  []string `json:"kube-proxy-arg,omitempty"`
	KubeProxyExtraMounts           []string `json:"kube-proxy-extra-mount,omitempty"`
	KubeProxyExtraEnvs             []string `json:"kube-proxy-extra-env,omitempty"`
	ProtectKernelDefaults          bool     `json:"protect-kernel-defaults,omitempty"`
	Selinux                        bool     `json:"selinux,omitempty"`
	LbServerPort                   string   `json:"lb-server-port,omitempty"`
	KubeProxyImage                 string   `json:"kube-proxy-image,omitempty"`
	PauseImage                     string   `json:"pause-image,omitempty"`
	RuntimeImage                   string   `json:"runtime-image,omitempty"`
	EtcdImage                      string   `json:"etcd-image,omitempty"`
	KubeletPath                    string   `json:"kubelet-path,omitempty"`
	CloudProviderName              string   `json:"cloud-provider-name,omitempty"`
	CloudProviderConfig            string   `json:"cloud-provider-config,omitempty"`
	Profile                        string   `json:"profile,omitempty"`
	AuditPolicyFile                string   `json:"audit-policy-file,omitempty"`
	PodSecurityAdmissionConfigFile string   `json:"pod-security-admission-config-file,omitempty"`
	ControlPlaneResourceRequests   []string `json:"control-plane-resource-requests,omitempty"`
	ControlPlaneResourceLimits     []string `json:"control-plane-resource-limits,omitempty"`
}

func GenerateInitControlPlaneConfig(controlPlaneEndpoint string, token string, serverConfig controlplanev1.RKE2ServerConfig, agentConfig bootstrapv1.RKE2AgentConfig) RKE2ServerConfig {
	rke2ServerConfig := RKE2ServerConfig{
		// TODO: Add data to the struct
		DisableCloudController: true,
		TLSSan:                 append(serverConfig.TLSSan, controlPlaneEndpoint),
		BindAddress:            serverConfig.BindAddress,
		AdvertiseAddress:       serverConfig.AdvertiseAddress,
		ClusterDNS:             serverConfig.ClusterDNS,
		ClusterDomain:          serverConfig.ClusterDomain,
	}

	rke2ServerConfig.RKE2AgentConfig = RKE2AgentConfig{
		Token:      token,
		NodeLabels: agentConfig.NodeLabels,
		NodeTaints: agentConfig.NodeTaints,
	}

	return rke2ServerConfig
}

func GenerateJoinControlPlaneConfig(serverUrl string, token string, controlplaneendpoint string, serverConfig controlplanev1.RKE2ServerConfig, agentConfig bootstrapv1.RKE2AgentConfig) RKE2ServerConfig {

	rke2ServerConfig := RKE2ServerConfig{
		// TODO: Add data to the struct
		DisableCloudController: true,
		TLSSan:                 append(serverConfig.TLSSan, controlplaneendpoint),
		BindAddress:            serverConfig.BindAddress,
		ClusterDNS:             serverConfig.ClusterDNS,
		ClusterDomain:          serverConfig.ClusterDomain,
	}

	rke2ServerConfig.RKE2AgentConfig = RKE2AgentConfig{
		// TODO : Add data to the struct
		Token:      token,
		Server:     serverUrl,
		NodeLabels: agentConfig.NodeLabels,
		NodeTaints: agentConfig.NodeTaints,
	}

	return rke2ServerConfig
}

func GenerateWorkerConfig(serverUrl string, token string, agentConfig bootstrapv1.RKE2AgentConfig) RKE2AgentConfig {
	return RKE2AgentConfig{
		// TODO : Add data to the struct
		Server:     serverUrl,
		Token:      token,
		NodeLabels: agentConfig.NodeLabels,
		NodeTaints: agentConfig.NodeTaints,
	}
}
