package controllers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/infrastructure"
	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/secret"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RKE2KubernetesVersion = "v1.34.2+rke2r1"
)

var _ = Describe("Rotate kubeconfig cert", func() {
	var (
		err              error
		ns               *corev1.Namespace
		rcp              *controlplanev1.RKE2ControlPlane
		caSecret         *corev1.Secret
		ccaSecret        *corev1.Secret
		kubeconfigSecret *corev1.Secret
		clusterKey       client.ObjectKey
	)
	BeforeEach(func() {
		kubeconfigSecret = &corev1.Secret{}
		ns, err = testEnv.CreateNamespace(ctx, "rotate-kubeconfig-cert")
		Expect(err).ToNot(HaveOccurred())
		clusterKey = client.ObjectKey{Namespace: ns.Name, Name: "rotate-kubeconfig-cert"}

		rcp = &controlplanev1.RKE2ControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: ns.Name,
				UID:       "foobar",
			},
			TypeMeta: metav1.TypeMeta{
				APIVersion: controlplanev1.GroupVersion.String(),
				Kind:       rke2ControlPlaneKind,
			},
		}

		// Generate new Secret Cluster CA
		certPEM, _, err := generateCertAndKey(time.Now().Add(3650 * 24 * time.Hour)) // 10 years from now
		Expect(err).ShouldNot(HaveOccurred())
		caSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret.Name(clusterKey.Name, secret.ClusterCA),
				Namespace: ns.Name,
			},
			StringData: map[string]string{
				secret.TLSCrtDataName: string(certPEM),
			},
		}
		Expect(testEnv.Client.Create(ctx, caSecret)).Should(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(caSecret), caSecret)).Should(Succeed())

		// Generate new Secret Client Cluster CA
		certPEM, keyPEM, err := generateCertAndKey(time.Now().Add(3650 * 24 * time.Hour)) // 10 years from now
		Expect(err).ShouldNot(HaveOccurred())
		ccaSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret.Name(clusterKey.Name, secret.ClientClusterCA),
				Namespace: ns.Name,
			},
			StringData: map[string]string{
				secret.TLSCrtDataName: string(certPEM),
				secret.TLSKeyDataName: string(keyPEM),
			},
		}
		Expect(testEnv.Client.Create(ctx, ccaSecret)).Should(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(ccaSecret), ccaSecret)).Should(Succeed())
	})

	AfterEach(func() {
		testEnv.Cleanup(ctx, kubeconfigSecret, ccaSecret, caSecret, ns)
	})
	It("Should rotate kubeconfig secret if needed", func() {
		By("Creating the first kubeconfig if not existing yet")
		r := &RKE2ControlPlaneReconciler{
			Client:                    testEnv.GetClient(),
			Scheme:                    testEnv.GetScheme(),
			managementCluster:         &rke2.Management{Client: testEnv.GetClient(), SecretCachingClient: testEnv.GetClient()},
			managementClusterUncached: &rke2.Management{Client: testEnv.GetClient()},
		}
		endpoint := clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}

		// Trigger first reconcile to generate a new Kubeconfig Secret
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(caSecret), caSecret)).Should(Succeed())
		_, err = r.reconcileKubeconfig(ctx, clusterKey, endpoint, rcp)
		Expect(err).ToNot(HaveOccurred())

		// Fetch the original Kubeconfig Secret
		Expect(testEnv.Get(ctx, types.NamespacedName{
			Namespace: ns.Name,
			Name:      secret.Name(clusterKey.Name, secret.Kubeconfig),
		}, kubeconfigSecret)).Should(Succeed())
		originalSecret := kubeconfigSecret.DeepCopy()

		By("Overriding the kubeconfig secret with short expiry")
		shortExpiryDate := time.Now().Add(24 * time.Hour) // 1 day from now
		Expect(updateKubeconfigSecret(kubeconfigSecret, shortExpiryDate)).Should(Succeed())
		Expect(testEnv.Update(ctx, kubeconfigSecret)).To(Succeed())

		By("Checking that rotation is needed")
		needsRotation, err := kubeconfig.NeedsClientCertRotation(kubeconfigSecret, certs.ClientCertificateRenewalDuration)
		Expect(err).ToNot(HaveOccurred())
		Expect(needsRotation).To(BeTrue())

		By("Rotating kubeconfig secret")
		_, err = r.reconcileKubeconfig(ctx, clusterKey, endpoint, rcp)
		Expect(err).ToNot(HaveOccurred())
		Expect(testEnv.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: kubeconfigSecret.Name}, kubeconfigSecret)).To(Succeed())
		Expect(kubeconfigSecret.StringData[secret.KubeconfigDataName]).Should(Equal(originalSecret.StringData[secret.KubeconfigDataName]), "Kubeconfig data must have been updated")

		By("Override the kubeconfig secret with a long expiry")
		longExpiryDate := time.Now().Add(365 * 24 * time.Hour) // 1 year from now
		Expect(updateKubeconfigSecret(kubeconfigSecret, longExpiryDate)).Should(Succeed())
		Expect(testEnv.Update(ctx, kubeconfigSecret)).To(Succeed())

		By("Checking that rotation is not needed")
		needsRotation, err = kubeconfig.NeedsClientCertRotation(kubeconfigSecret, certs.ClientCertificateRenewalDuration)
		Expect(err).ToNot(HaveOccurred())
		Expect(needsRotation).To(BeFalse())

		By("Fetching the overridden kubeconfig Secret")
		Expect(testEnv.Get(ctx, types.NamespacedName{
			Namespace: ns.Name,
			Name:      secret.Name(clusterKey.Name, secret.Kubeconfig),
		}, kubeconfigSecret)).Should(Succeed())
		updatedSecret := kubeconfigSecret.DeepCopy()

		By("Ensuring no rotation occurs")
		_, err = r.reconcileKubeconfig(ctx, clusterKey, endpoint, rcp)
		Expect(err).ToNot(HaveOccurred())

		Expect(testEnv.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: kubeconfigSecret.Name}, kubeconfigSecret)).To(Succeed())
		Expect(kubeconfigSecret.StringData[secret.KubeconfigDataName]).Should(Equal(updatedSecret.StringData[secret.KubeconfigDataName]), "Kubeconfig data must stay the same")
	})
})

var _ = Describe("Reconcile control plane conditions", func() {
	var (
		err            error
		cp             *rke2.ControlPlane
		rcp            *controlplanev1.RKE2ControlPlane
		ns             *corev1.Namespace
		nodeName       = "node1"
		node           *corev1.Node
		nodeByRef      *corev1.Node
		orphanedNode   *corev1.Node
		machine        *clusterv1.Machine
		machineWithRef *clusterv1.Machine
		config         *bootstrapv1.RKE2Config
	)

	BeforeEach(func() {
		ns, err = testEnv.CreateNamespace(ctx, "ns")
		Expect(err).ToNot(HaveOccurred())

		annotations := map[string]string{
			"test": "true",
		}

		config = &bootstrapv1.RKE2Config{ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: ns.Name,
		}, Spec: bootstrapv1.RKE2ConfigSpec{
			AgentConfig: bootstrapv1.RKE2AgentConfig{
				NodeAnnotations: annotations,
			},
		}}
		Expect(testEnv.Create(ctx, config)).To(Succeed())

		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "true",
				},
				Annotations: map[string]string{
					clusterv1.MachineAnnotation: nodeName,
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Reason: "Node ready",
					Status: corev1.ConditionTrue,
				}},
			},
		}
		Expect(testEnv.Create(ctx, node.DeepCopy())).To(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), node)).Should(Succeed())
		Expect(testEnv.Status().Update(ctx, node.DeepCopy())).To(Succeed())

		nodeRefName := "ref-node"
		machineWithRef = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-with-ref",
				Namespace: ns.Name,
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "cluster",
				Version:     RKE2KubernetesVersion,
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "RKE2Config",
						APIGroup: bootstrapv1.GroupVersion.Group,
						Name:     config.Name,
					},
				},
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "FakeMachine",
					APIGroup: infrastructure.GroupVersion.Group,
					Name:     "fakemref1",
				},
			},
			Status: clusterv1.MachineStatus{
				NodeRef: clusterv1.MachineNodeReference{
					Name: nodeRefName,
				},
				Conditions: []metav1.Condition{
					{
						Type:               clusterv1.MachineReadyCondition,
						Reason:             clusterv1.MachineReadyReason,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				},
			},
		}

		ml := clusterv1.MachineList{Items: []clusterv1.Machine{*machineWithRef.DeepCopy()}}
		updatedMachine := machineWithRef.DeepCopy()
		Expect(testEnv.Create(ctx, updatedMachine)).To(Succeed())
		updatedMachine.Status = *machineWithRef.Status.DeepCopy()
		machineWithRef = updatedMachine.DeepCopy()
		Expect(testEnv.Status().Update(ctx, machineWithRef)).To(Succeed())

		nodeByRef = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeRefName,
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "true",
				},
				Annotations: map[string]string{
					clusterv1.MachineAnnotation: machineWithRef.Name,
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Reason: "Node ready",
					Status: corev1.ConditionTrue,
				}},
			},
		}
		Expect(testEnv.Create(ctx, nodeByRef.DeepCopy())).To(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(nodeByRef), nodeByRef)).Should(Succeed())
		Expect(testEnv.Status().Update(ctx, nodeByRef.DeepCopy())).To(Succeed())

		orphanedNode = &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: "missing-machine",
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "true",
			},
		}}
		Expect(testEnv.Create(ctx, orphanedNode)).To(Succeed())

		machine = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nodeName,
				Namespace: ns.Name,
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "cluster",
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "RKE2Config",
						APIGroup: bootstrapv1.GroupVersion.Group,
						Name:     config.Name,
					},
				},
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "FakeMachine",
					APIGroup: infrastructure.GroupVersion.Group,
					Name:     "fakem1",
				},
			},
			Status: clusterv1.MachineStatus{
				NodeRef: clusterv1.MachineNodeReference{
					Name: nodeName,
				},
				Conditions: []metav1.Condition{
					{
						Type:               clusterv1.MachineReadyCondition,
						Reason:             clusterv1.MachineReadyReason,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				},
			},
		}

		ml.Items = append(ml.Items, *machine.DeepCopy())
		updatedMachine = machine.DeepCopy()
		Expect(testEnv.Create(ctx, updatedMachine)).To(Succeed())
		updatedMachine.Status = *machine.Status.DeepCopy()
		machine = updatedMachine.DeepCopy()
		Expect(testEnv.Status().Update(ctx, machine)).To(Succeed())

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: ns.Name,
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{},
				ClusterNetwork: clusterv1.ClusterNetwork{
					Pods: clusterv1.NetworkRanges{
						CIDRBlocks: []string{
							"192.168.0.0/16",
						},
					},
					Services: clusterv1.NetworkRanges{
						CIDRBlocks: []string{
							"192.169.0.0/16",
						},
					},
				},
			},
		}
		Expect(testEnv.Client.Create(ctx, cluster)).To(Succeed())

		rcp = &controlplanev1.RKE2ControlPlane{
			Status: controlplanev1.RKE2ControlPlaneStatus{
				Initialization: controlplanev1.RKE2ControlPlaneInitializationStatus{
					ControlPlaneInitialized: ptr.To(true),
				},
				Conditions: []metav1.Condition{
					{
						Type:    controlplanev1.RKE2ControlPlaneInitializedCondition,
						Status:  metav1.ConditionTrue,
						Reason:  controlplanev1.RKE2ControlPlaneInitializedReason,
						Message: "",
					},
				},
			},
		}

		m := &rke2.Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		cp, err = rke2.NewControlPlane(ctx, m, testEnv.GetClient(), cluster, rcp, collections.FromMachineList(&ml))
		Expect(err).ToNot(HaveOccurred())

		ref := metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       clusterv1.ClusterKind,
			UID:        cp.Cluster.GetUID(),
			Name:       cp.Cluster.GetName(),
		}
		Expect(testEnv.Client.Create(ctx, kubeconfig.GenerateSecretWithOwner(
			client.ObjectKeyFromObject(cp.Cluster),
			kubeconfig.FromEnvTestConfig(testEnv.Config, cp.Cluster),
			ref))).To(Succeed())
	})

	AfterEach(func() {
		Expect(testEnv.DeleteAllOf(ctx, node)).To(Succeed())
		testEnv.Cleanup(ctx, node, ns)
	})

	It("should reconcile cp and machine conditions successfully", func() {
		r := &RKE2ControlPlaneReconciler{
			Client:                    testEnv.GetClient(),
			Scheme:                    testEnv.GetScheme(),
			managementCluster:         &rke2.Management{Client: testEnv.GetClient(), SecretCachingClient: testEnv.GetClient()},
			managementClusterUncached: &rke2.Management{Client: testEnv.GetClient()},
		}
		_, err := r.reconcileControlPlaneConditions(ctx, cp)
		Expect(err).ToNot(HaveOccurred())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(machine), machine)).To(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(machineWithRef), machineWithRef)).To(Succeed())
		Expect(conditions.IsTrue(machine, controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(BeTrue())
		Expect(conditions.IsTrue(machineWithRef, controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(BeTrue())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), node)).To(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(nodeByRef), nodeByRef)).To(Succeed())
		Expect(node.GetAnnotations()).To(HaveKeyWithValue("test", "true"))
		Expect(nodeByRef.GetAnnotations()).To(HaveKeyWithValue("test", "true"))
		Expect(conditions.IsFalse(rcp, controlplanev1.RKE2ControlPlaneControlPlaneComponentsHealthyCondition)).To(BeTrue())
		Expect(conditions.GetMessage(rcp, controlplanev1.RKE2ControlPlaneControlPlaneComponentsHealthyCondition)).To(Equal(
			"Control plane node missing-machine does not have a corresponding machine"))
	})
})

// generateCertAndKey generates a self-signed certificate and private key.
func generateCertAndKey(expiryDate time.Time) ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		NotBefore: time.Now(),
		NotAfter:  expiryDate,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return certPEM, keyPEM, nil
}

// updateKubeconfigSecret updates a Kubernetes secret with a kubeconfig containing a client certificate and key.
func updateKubeconfigSecret(configSecret *corev1.Secret, expiryDate time.Time) error {
	certPEM, keyPEM, err := generateCertAndKey(expiryDate)
	if err != nil {
		return fmt.Errorf("Generating Cert and Key: %w", err)
	}

	configSecret.Data[secret.KubeconfigDataName] = []byte(fmt.Sprintf(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://1.2.3.4:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    client-certificate-data: %s
    client-key-data: %s
`, base64.StdEncoding.EncodeToString(certPEM), base64.StdEncoding.EncodeToString(keyPEM)))

	return nil
}
