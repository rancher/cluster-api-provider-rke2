/*
Copyright 2023 SUSE.
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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/cluster-api/api/v1beta1"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher/cluster-api-provider-rke2/pkg/consts"
)

const (
	expEncryptionConfig = `{"kind":"EncryptionConfiguration","apiVersion":"apiserver.config.k8s.io/v1","resources":[{"resources":["secrets"],"providers":[{"secretbox":{"keys":[{"name":"enckey","secret":"dGVzdF9lbmNyeXB0aW9uX2tleQ=="}]}},{"identity":{}}]}]}`
)

var _ = Describe("RKE2ServerConfig", func() {
	var opts *ServerConfigOpts

	BeforeEach(func() {
		opts = &ServerConfigOpts{
			Token: "just-a-test-token",
			Cluster: v1beta1.Cluster{
				Spec: v1beta1.ClusterSpec{
					ClusterNetwork: &v1beta1.ClusterNetwork{
						Pods: &v1beta1.NetworkRanges{
							CIDRBlocks: []string{
								"192.168.0.0/16",
							},
						},
						Services: &v1beta1.NetworkRanges{
							CIDRBlocks: []string{
								"192.169.0.0/16",
							},
						},
					},
				},
			},
			ControlPlaneEndpoint: "testendpoint",
			Ctx:                  context.Background(),
			Client: fake.NewClientBuilder().WithObjects(
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"aws_access_key_id":     []byte("test_id"),
						"aws_secret_access_key": []byte("test_secret"),
						"ca.pem":                []byte("test_ca"),
						"audit-policy.yaml":     []byte("test_audit"),
						"endpoint":              []byte("test_endpoint"),
						"cert.pem":              []byte("test_cert"),
						"key.pem":               []byte("test_key"),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string]string{
						"cloud-config":                 "test_cloud_config",
						"credential-config.yaml":       "test_credential_config",
						"credential-provider-binaries": "test_credential_provider_binaries",
						"resolv.conf":                  "test_resolv_conf",
					},
				},
			).Build(),
			ServerConfig: controlplanev1.RKE2ServerConfig{
				AdvertiseAddress: "testaddress",
				AuditPolicySecret: &corev1.ObjectReference{
					Name:      "test",
					Namespace: "test",
				},

				BindAddress:   "testbindaddress",
				CNI:           controlplanev1.Cilium,
				ClusterDNS:    "testdns",
				ClusterDomain: "testdomain",
				CloudProviderConfigMap: &corev1.ObjectReference{
					Name:      "test",
					Namespace: "test",
				},
				CloudProviderName: "testcloud",
				DisableComponents: controlplanev1.DisableComponents{
					PluginComponents: []controlplanev1.DisabledPluginComponent{
						controlplanev1.CoreDNS,
					},
					KubernetesComponents: []controlplanev1.DisabledKubernetesComponent{
						controlplanev1.KubeProxy,
						controlplanev1.Scheduler,
						controlplanev1.CloudController,
					},
				},
				Etcd: controlplanev1.EtcdConfig{
					ExposeMetrics: true,
					BackupConfig: controlplanev1.EtcdBackupConfig{
						S3: &controlplanev1.EtcdS3{
							S3CredentialSecret: &corev1.ObjectReference{
								Name:      "test",
								Namespace: "test",
							},
							EndpointCASecret: &corev1.ObjectReference{
								Name:      "test",
								Namespace: "test",
							},
							Bucket:           "testbucket",
							Region:           "testregion",
							Endpoint:         "testendpoint",
							EnforceSSLVerify: true,
						},
						Directory:    "testdir",
						SnapshotName: "testsnapshot",
						Retention:    "testretention",
						ScheduleCron: "testschedule",
					},
					CustomConfig: &bootstrapv1.ComponentConfig{
						ExtraArgs:     []string{"testarg"},
						OverrideImage: "testimage",
						ExtraMounts:   map[string]string{"testmount": "testmount"},
						ExtraEnv:      map[string]string{"testenv": "testenv"},
					},
				},
				ServiceNodePortRange: "testrange",
				TLSSan:               []string{"testsan"},
				KubeAPIServer: &bootstrapv1.ComponentConfig{
					ExtraArgs:     []string{"testarg"},
					OverrideImage: "testimage",
					ExtraMounts:   map[string]string{"testmount": "testmount"},
					ExtraEnv:      map[string]string{"testenv": "testenv"},
				},
				KubeScheduler: &bootstrapv1.ComponentConfig{
					ExtraArgs:     []string{"testarg"},
					OverrideImage: "testimage",
					ExtraMounts:   map[string]string{"testmount": "testmount"},
					ExtraEnv:      map[string]string{"testenv": "testenv"},
				},
				KubeControllerManager: &bootstrapv1.ComponentConfig{
					ExtraArgs:     []string{"testarg"},
					OverrideImage: "testimage",
					ExtraEnv:      map[string]string{"testenv": "testenv"},
					ExtraMounts:   map[string]string{"testmount": "testmount"},
				},
				CloudControllerManager: &bootstrapv1.ComponentConfig{
					ExtraArgs:     []string{"testarg"},
					OverrideImage: "testimage",
					ExtraEnv:      map[string]string{"testenv": "testenv"},
					ExtraMounts:   map[string]string{"testmount": "testmount"},
				},
				EmbeddedRegistry: true,
				ExternalDatastoreSecret: &corev1.ObjectReference{
					Name:      "test",
					Namespace: "test",
				},
			},
		}
	})

	It("should succefully generate a server config", func() {
		rke2ServerConfig, files, err := GenerateInitControlPlaneConfig(*opts)
		Expect(err).ToNot(HaveOccurred())

		serverConfig := opts.ServerConfig
		Expect(rke2ServerConfig.AdvertiseAddress).To(Equal(serverConfig.AdvertiseAddress))
		Expect(rke2ServerConfig.AuditPolicyFile).To(Equal("/etc/rancher/rke2/audit-policy.yaml"))
		Expect(rke2ServerConfig.BindAddress).To(Equal(serverConfig.BindAddress))
		Expect(rke2ServerConfig.CNI).To(Equal([]string{string(serverConfig.CNI)}))
		Expect(rke2ServerConfig.ClusterCIDR).To(Equal("192.168.0.0/16"))
		Expect(rke2ServerConfig.ServiceCIDR).To(Equal("192.169.0.0/16"))
		Expect(rke2ServerConfig.ClusterDNS).To(Equal(serverConfig.ClusterDNS))
		Expect(rke2ServerConfig.ClusterDomain).To(Equal(serverConfig.ClusterDomain))
		Expect(rke2ServerConfig.CloudProviderConfig).To(Equal("/etc/rancher/rke2/cloud-provider-config"))
		Expect(rke2ServerConfig.CloudProviderName).To(Equal(opts.ServerConfig.CloudProviderName))
		Expect(rke2ServerConfig.DisableComponents).To(Equal([]string{string(controlplanev1.CoreDNS)}))
		Expect(rke2ServerConfig.DisableKubeProxy).To(BeTrue())
		Expect(rke2ServerConfig.DisableCloudController).To(BeTrue())
		Expect(rke2ServerConfig.DisableScheduler).To(BeTrue())
		// Expect(rke2ServerConfig.EtcdDisableSnapshots).To(BeFalse())
		Expect(rke2ServerConfig.EtcdExposeMetrics).To(Equal(serverConfig.Etcd.ExposeMetrics))
		Expect(rke2ServerConfig.EtcdS3).To(BeTrue())
		Expect(rke2ServerConfig.EtcdS3AccessKey).To(Equal("test_id"))
		Expect(rke2ServerConfig.EtcdS3SecretKey).To(Equal("test_secret"))
		Expect(rke2ServerConfig.EtcdS3Bucket).To(Equal(serverConfig.Etcd.BackupConfig.S3.Bucket))
		Expect(rke2ServerConfig.EtcdS3Region).To(Equal(serverConfig.Etcd.BackupConfig.S3.Region))
		Expect(rke2ServerConfig.EtcdS3Folder).To(Equal(serverConfig.Etcd.BackupConfig.S3.Folder))
		Expect(rke2ServerConfig.EtcdS3Endpoint).To(Equal(serverConfig.Etcd.BackupConfig.S3.Endpoint))
		Expect(rke2ServerConfig.EtcdS3EndpointCA).To(Equal("/etc/rancher/rke2/etcd-s3-ca.crt"))
		Expect(rke2ServerConfig.EtcdSnapshotDir).To(Equal(serverConfig.Etcd.BackupConfig.Directory))
		Expect(rke2ServerConfig.EtcdSnapshotName).To(Equal(serverConfig.Etcd.BackupConfig.SnapshotName))
		Expect(rke2ServerConfig.EtcdSnapshotRetention).To(Equal(serverConfig.Etcd.BackupConfig.Retention))
		Expect(rke2ServerConfig.EtcdSnapshotScheduleCron).To(Equal(serverConfig.Etcd.BackupConfig.ScheduleCron))
		Expect(rke2ServerConfig.EtcdS3SkipSslVerify).To(BeFalse())
		Expect(rke2ServerConfig.EtcdArgs).To(Equal(serverConfig.Etcd.CustomConfig.ExtraArgs))
		Expect(rke2ServerConfig.EtcdImage).To(Equal(serverConfig.Etcd.CustomConfig.OverrideImage))
		Expect(rke2ServerConfig.EtcdExtraMounts).To(Equal(componentMapToSlice(extraMount, serverConfig.Etcd.CustomConfig.ExtraMounts)))
		Expect(rke2ServerConfig.EtcdExtraEnv).To(Equal(componentMapToSlice(extraEnv, serverConfig.Etcd.CustomConfig.ExtraEnv)))
		Expect(rke2ServerConfig.ServiceNodePortRange).To(Equal(rke2ServerConfig.ServiceNodePortRange))
		Expect(rke2ServerConfig.TLSSan).To(Equal(append(serverConfig.TLSSan, opts.ControlPlaneEndpoint)))
		Expect(rke2ServerConfig.KubeAPIServerArgs).To(Equal(serverConfig.KubeAPIServer.ExtraArgs))
		Expect(rke2ServerConfig.KubeAPIserverImage).To(Equal(serverConfig.KubeAPIServer.OverrideImage))
		Expect(rke2ServerConfig.KubeAPIserverExtraMounts).To(Equal(componentMapToSlice(extraMount, serverConfig.KubeAPIServer.ExtraMounts)))
		Expect(rke2ServerConfig.KubeAPIserverExtraEnv).To(Equal(componentMapToSlice(extraEnv, serverConfig.KubeAPIServer.ExtraEnv)))
		Expect(rke2ServerConfig.KubeSchedulerArgs).To(Equal(serverConfig.KubeScheduler.ExtraArgs))
		Expect(rke2ServerConfig.KubeSchedulerImage).To(Equal(serverConfig.KubeScheduler.OverrideImage))
		Expect(rke2ServerConfig.KubeSchedulerExtraMounts).To(Equal(componentMapToSlice(extraMount, serverConfig.KubeScheduler.ExtraMounts)))
		Expect(rke2ServerConfig.KubeSchedulerExtraEnv).To(Equal(componentMapToSlice(extraEnv, serverConfig.KubeScheduler.ExtraEnv)))
		Expect(rke2ServerConfig.KubeControllerManagerArgs).To(Equal(serverConfig.KubeControllerManager.ExtraArgs))
		Expect(rke2ServerConfig.KubeControllerManagerImage).To(Equal(serverConfig.KubeControllerManager.OverrideImage))
		Expect(rke2ServerConfig.KubeControllerManagerExtraMounts).To(Equal(componentMapToSlice(extraMount, serverConfig.KubeControllerManager.ExtraMounts)))
		Expect(rke2ServerConfig.KubeControllerManagerExtraEnv).To(Equal(componentMapToSlice(extraEnv, serverConfig.KubeControllerManager.ExtraEnv)))
		Expect(rke2ServerConfig.CloudControllerManagerExtraMounts).To(Equal(componentMapToSlice(extraMount, serverConfig.CloudControllerManager.ExtraMounts)))
		Expect(rke2ServerConfig.CloudControllerManagerExtraEnv).To(Equal(componentMapToSlice(extraEnv, serverConfig.CloudControllerManager.ExtraEnv)))
		Expect(rke2ServerConfig.Token).To(Equal(opts.Token))
		Expect(rke2ServerConfig.EmbeddedRegistry).To(BeTrue())
		Expect(rke2ServerConfig.DatastoreEndpoint).To(Equal("test_endpoint"))
		Expect(rke2ServerConfig.DatastoreCAFile).To(Equal("/etc/rancher/rke2/datastore-ca.crt"))
		Expect(rke2ServerConfig.DatastoreCertFile).To(Equal("/etc/rancher/rke2/datastore-cert.crt"))
		Expect(rke2ServerConfig.DatastoreKeyFile).To(Equal("/etc/rancher/rke2/datastore-key.crt"))

		Expect(files).To(HaveLen(7))

		Expect(files[0].Path).To(Equal(rke2ServerConfig.AuditPolicyFile))
		Expect(files[0].Content).To(Equal("test_audit"))
		Expect(files[0].Owner).To(Equal(consts.DefaultFileOwner))
		Expect(files[0].Permissions).To(Equal(consts.DefaultFileMode))

		Expect(files[1].Path).To(Equal(rke2ServerConfig.CloudProviderConfig))
		Expect(files[1].Content).To(Equal("test_cloud_config"))
		Expect(files[1].Owner).To(Equal(consts.DefaultFileOwner))
		Expect(files[1].Permissions).To(Equal(consts.DefaultFileMode))

		Expect(files[2].Path).To(Equal(rke2ServerConfig.EtcdS3EndpointCA))
		Expect(files[2].Content).To(Equal("test_ca"))
		Expect(files[2].Owner).To(Equal(consts.DefaultFileOwner))
		Expect(files[2].Permissions).To(Equal("0640"))

		Expect(files[3].Path).To(Equal(rke2ServerConfig.DatastoreCAFile))
		Expect(files[3].Content).To(Equal("test_ca"))
		Expect(files[3].Owner).To(Equal(consts.DefaultFileOwner))
		Expect(files[3].Permissions).To(Equal("0640"))

		Expect(files[4].Path).To(Equal(rke2ServerConfig.DatastoreCertFile))
		Expect(files[4].Content).To(Equal("test_cert"))
		Expect(files[4].Owner).To(Equal(consts.DefaultFileOwner))
		Expect(files[4].Permissions).To(Equal("0640"))

		Expect(files[5].Path).To(Equal(rke2ServerConfig.DatastoreKeyFile))
		Expect(files[5].Content).To(Equal("test_key"))
		Expect(files[5].Owner).To(Equal(consts.DefaultFileOwner))
		Expect(files[5].Permissions).To(Equal("0640"))

		Expect(files[6].Path).To(Equal(rke2ServerConfig.CloudProviderConfig))
		Expect(files[6].Content).To(Equal("test_cloud_config"))
		Expect(files[6].Owner).To(Equal(consts.DefaultFileOwner))
		Expect(files[6].Permissions).To(Equal(consts.DefaultFileMode))
	})
})

var _ = Describe("RKE2 Server Config with secretbox encryption", func() {
	var opts *ServerConfigOpts

	BeforeEach(func() {

		opts = &ServerConfigOpts{
			Token: "token",
			Cluster: v1beta1.Cluster{
				Spec: v1beta1.ClusterSpec{
					ClusterNetwork: &v1beta1.ClusterNetwork{
						Pods: &v1beta1.NetworkRanges{
							CIDRBlocks: []string{
								"192.168.0.0/16",
							},
						},
						Services: &v1beta1.NetworkRanges{
							CIDRBlocks: []string{
								"192.169.0.0/16",
							},
						},
					},
				},
			},
			ControlPlaneEndpoint: "testendpoint",
			Ctx:                  context.Background(),
			Client: fake.NewClientBuilder().WithObjects(
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "encryption-key",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"encryptionKey": []byte("test_encryption_key"),
					},
				},
			).Build(),
			ServerConfig: controlplanev1.RKE2ServerConfig{
				BindAddress:   "testbindaddress",
				CNI:           controlplanev1.Cilium,
				ClusterDNS:    "testdns",
				ClusterDomain: "testdomain",
				SecretsEncryptionProvider: &controlplanev1.SecretsEncryption{
					Provider: "secretbox",
					EncryptionKeySecret: &corev1.ObjectReference{
						Name:      "encryption-key",
						Namespace: "test",
					},
				},
			},
		}
	})

	It("should succefully generate a server config with secretbox key provider", func() {
		rke2ServerConfig, files, err := GenerateInitControlPlaneConfig(*opts)
		Expect(err).ToNot(HaveOccurred())

		Expect(rke2ServerConfig.SecretsEncryptionProvider).To(Equal("secretbox"))
		Expect(files[0].Content).To(Equal(expEncryptionConfig))
	})
})

var _ = Describe("RKE2 Agent Config", func() {
	var opts *AgentConfigOpts

	BeforeEach(func() {
		opts = &AgentConfigOpts{
			ServerURL: "testurl",
			Ctx:       context.Background(),
			Token:     "testtoken",
			Client: fake.NewClientBuilder().WithObjects(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string]string{
						"credential-config.yaml":       "test_credential_config",
						"credential-provider-binaries": "test_credential_provider_binaries",
						"resolv.conf":                  "test_resolv_conf",
					},
				},
			).Build(),
			AgentConfig: bootstrapv1.RKE2AgentConfig{
				ContainerRuntimeEndpoint: "testendpoint",
				DataDir:                  "testdir",
				ImageCredentialProviderConfigMap: &corev1.ObjectReference{
					Name:      "test",
					Namespace: "test",
				},
				SystemDefaultRegistry: "testregistry",
				KubeletPath:           "testpath",
				Kubelet: &bootstrapv1.ComponentConfig{
					ExtraArgs: []string{"testarg"},
				},
				LoadBalancerPort:      1234,
				NodeLabels:            []string{"testlabel"},
				NodeTaints:            []string{"testtaint"},
				CISProfile:            bootstrapv1.CIS, //nolint:nosnakecase
				ProtectKernelDefaults: true,
				ResolvConf: &corev1.ObjectReference{
					Name:      "test",
					Namespace: "test",
				},
				RuntimeImage:            "testimage",
				EnableContainerdSElinux: true,
				Snapshotter:             "testsnapshotter",
				KubeProxy: &bootstrapv1.ComponentConfig{
					ExtraArgs:     []string{"testarg"},
					OverrideImage: "testimage",
					ExtraEnv:      map[string]string{"testenv": "testenv"},
					ExtraMounts:   map[string]string{"testmount": "testmount"},
				},
			},
			Version: "v1.25.2",
		}
	})

	It("should successfully generate an agent config", func() {
		agentConfig, files, err := GenerateWorkerConfig(*opts)
		Expect(err).ToNot(HaveOccurred())

		Expect(agentConfig.ContainerRuntimeEndpoint).To(Equal(opts.AgentConfig.ContainerRuntimeEndpoint))
		Expect(agentConfig.DataDir).To(Equal(opts.AgentConfig.DataDir))
		Expect(agentConfig.ImageCredentialProviderConfig).To(Equal("/etc/rancher/rke2/credential-config.yaml"))
		Expect(agentConfig.ImageCredentialProviderBinDir).To(Equal("test_credential_provider_binaries"))
		Expect(agentConfig.KubeletPath).To(Equal(opts.AgentConfig.KubeletPath))
		Expect(agentConfig.KubeletArgs).To(Equal(opts.AgentConfig.Kubelet.ExtraArgs))
		Expect(agentConfig.LbServerPort).To(Equal(opts.AgentConfig.LoadBalancerPort))
		Expect(agentConfig.NodeLabels).To(Equal(opts.AgentConfig.NodeLabels))
		Expect(agentConfig.NodeTaints).To(Equal(opts.AgentConfig.NodeTaints))
		Expect(agentConfig.Profile).To(Equal(string(opts.AgentConfig.CISProfile)))
		Expect(agentConfig.ProtectKernelDefaults).To(Equal(opts.AgentConfig.ProtectKernelDefaults))
		Expect(agentConfig.ResolvConf).To(Equal("/etc/rancher/rke2/resolv.conf"))
		Expect(agentConfig.RuntimeImage).To(Equal(opts.AgentConfig.RuntimeImage))
		Expect(agentConfig.Selinux).To(Equal(opts.AgentConfig.EnableContainerdSElinux))
		Expect(agentConfig.Server).To(Equal(opts.ServerURL))
		Expect(agentConfig.Snapshotter).To(Equal(opts.AgentConfig.Snapshotter))
		Expect(agentConfig.KubeProxyArgs).To(Equal(opts.AgentConfig.KubeProxy.ExtraArgs))
		Expect(agentConfig.KubeProxyImage).To(Equal(opts.AgentConfig.KubeProxy.OverrideImage))
		Expect(agentConfig.KubeProxyExtraMounts).To(Equal(componentMapToSlice(extraMount, opts.AgentConfig.KubeProxy.ExtraMounts)))
		Expect(agentConfig.KubeProxyExtraEnv).To(Equal(componentMapToSlice(extraEnv, opts.AgentConfig.KubeProxy.ExtraEnv)))
		Expect(agentConfig.Token).To(Equal(opts.Token))
		Expect(agentConfig.SystemDefaultRegistry).To(Equal(opts.AgentConfig.SystemDefaultRegistry))

		Expect(files).To(HaveLen(3))

		Expect(files[1].Path).To(Equal(agentConfig.ImageCredentialProviderConfig))
		Expect(files[1].Content).To(Equal("test_credential_config"))
		Expect(files[1].Owner).To(Equal(consts.DefaultFileOwner))
		Expect(files[1].Permissions).To(Equal(consts.DefaultFileMode))

		Expect(files[2].Path).To(Equal(agentConfig.ResolvConf))
		Expect(files[2].Content).To(Equal("test_resolv_conf"))
		Expect(files[2].Owner).To(Equal(consts.DefaultFileOwner))
		Expect(files[2].Permissions).To(Equal(consts.DefaultFileMode))
	})
})

var _ = Describe("componentMapToSlice", func() {
	It("should convert a single key-value pair map to a slice with '=' separator for extraEnv", func() {
		input := map[string]string{"FOO": "BAR"}
		expected := []string{"FOO=BAR"}
		Expect(componentMapToSlice(extraEnv, input)).To(Equal(expected))
	})

	It("should convert a single key-value pair map to a slice with ':' separator for extraMount", func() {
		input := map[string]string{"FOO": "BAR"}
		expected := []string{"FOO:BAR"}
		Expect(componentMapToSlice(extraMount, input)).To(Equal(expected))
	})

	It("should handle multiple key-value pairs with '=' separator for extraEnv", func() {
		input := map[string]string{"FOO": "BAR", "HELLO": "WORLD"}
		result := componentMapToSlice(extraEnv, input)
		Expect(result).To(ContainElements("FOO=BAR", "HELLO=WORLD"))
		Expect(len(result)).To(Equal(2))
	})

	It("should handle multiple key-value pairs with ':' separator for extraMount", func() {
		input := map[string]string{"FOO": "BAR", "HELLO": "WORLD"}
		result := componentMapToSlice(extraMount, input)
		Expect(result).To(ContainElements("FOO:BAR", "HELLO:WORLD"))
		Expect(len(result)).To(Equal(2))
	})

	It("should return an empty slice for an empty map", func() {
		input := map[string]string{}
		expected := []string{}
		Expect(componentMapToSlice(extraEnv, input)).To(Equal(expected))
		Expect(componentMapToSlice(extraMount, input)).To(Equal(expected))
	})

	It("should handle maps with empty values for extraEnv", func() {
		input := map[string]string{"FOO": ""}
		expected := []string{"FOO="}
		Expect(componentMapToSlice(extraEnv, input)).To(Equal(expected))
	})

	It("should handle maps with empty values for extraMount", func() {
		input := map[string]string{"FOO": ""}
		expected := []string{"FOO:"}
		Expect(componentMapToSlice(extraMount, input)).To(Equal(expected))
	})

	It("should skip entries with empty keys for extraEnv", func() {
		input := map[string]string{"": "BAR", "FOO": "BAR"}
		expected := []string{"FOO=BAR"}
		Expect(componentMapToSlice(extraEnv, input)).To(Equal(expected))
	})

	It("should skip entries with empty keys for extraMount", func() {
		input := map[string]string{"": "BAR", "FOO": "BAR"}
		expected := []string{"FOO:BAR"}
		Expect(componentMapToSlice(extraMount, input)).To(Equal(expected))
	})

	It("should skip entries with empty keys and values for extraEnv", func() {
		input := map[string]string{"": "", "FOO": "BAR"}
		expected := []string{"FOO=BAR"}
		Expect(componentMapToSlice(extraEnv, input)).To(Equal(expected))
	})

	It("should skip entries with empty keys and values for extraMount", func() {
		input := map[string]string{"": "", "FOO": "BAR"}
		expected := []string{"FOO:BAR"}
		Expect(componentMapToSlice(extraMount, input)).To(Equal(expected))
	})

	It("should skip entries with empty keys even if values are non-empty for extraEnv", func() {
		input := map[string]string{"": "NON_EMPTY", "FOO": "BAR"}
		expected := []string{"FOO=BAR"}
		Expect(componentMapToSlice(extraEnv, input)).To(Equal(expected))
	})

	It("should skip entries with empty keys even if values are non-empty for extraMount", func() {
		input := map[string]string{"": "NON_EMPTY", "FOO": "BAR"}
		expected := []string{"FOO:BAR"}
		Expect(componentMapToSlice(extraMount, input)).To(Equal(expected))
	})
})
