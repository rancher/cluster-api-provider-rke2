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
	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
					"node-role.kubernetes.io/master": "true",
				},
				Annotations: map[string]string{
					clusterv1.MachineAnnotation: nodeName,
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				}},
			},
		}
		Expect(testEnv.Create(ctx, node.DeepCopy())).To(Succeed())
		Eventually(testEnv.Get(ctx, client.ObjectKeyFromObject(node), node)).Should(Succeed())
		Expect(testEnv.Status().Update(ctx, node.DeepCopy())).To(Succeed())

		nodeRefName := "ref-node"
		machineWithRef = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-with-ref",
				Namespace: ns.Name,
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "cluster",
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "RKE2Config",
						APIVersion: bootstrapv1.GroupVersion.String(),
						Name:       config.Name,
						Namespace:  config.Namespace,
					},
				},
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "Pod",
					APIVersion: "v1",
					Name:       "stub",
					Namespace:  ns.Name,
				},
			},
			Status: clusterv1.MachineStatus{
				NodeRef: &corev1.ObjectReference{
					Kind:       "Node",
					APIVersion: "v1",
					Name:       nodeRefName,
				},
				Conditions: clusterv1.Conditions{
					clusterv1.Condition{
						Type:               clusterv1.ReadyCondition,
						Status:             corev1.ConditionTrue,
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
					"node-role.kubernetes.io/master": "true",
				},
				Annotations: map[string]string{
					clusterv1.MachineAnnotation: machineWithRef.Name,
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				}},
			},
		}
		Expect(testEnv.Create(ctx, nodeByRef.DeepCopy())).To(Succeed())
		Eventually(func() error {
			return testEnv.Get(ctx, client.ObjectKeyFromObject(nodeByRef), nodeByRef)
		}, 5*time.Second).Should(Succeed())
		Expect(testEnv.Status().Update(ctx, nodeByRef.DeepCopy())).To(Succeed())

		orphanedNode = &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: "missing-machine",
			Labels: map[string]string{
				"node-role.kubernetes.io/master": "true",
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
					ConfigRef: &corev1.ObjectReference{
						Kind:       "RKE2Config",
						APIVersion: bootstrapv1.GroupVersion.String(),
						Name:       config.Name,
						Namespace:  config.Namespace,
					},
				},
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "Pod",
					APIVersion: "v1",
					Name:       "stub",
					Namespace:  ns.Name,
				},
			},
			Status: clusterv1.MachineStatus{
				NodeRef: &corev1.ObjectReference{
					Kind:      "Node",
					Name:      nodeName,
					UID:       node.GetUID(),
					Namespace: "",
				},
				Conditions: clusterv1.Conditions{
					clusterv1.Condition{
						Type:               clusterv1.ReadyCondition,
						Status:             corev1.ConditionTrue,
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
		}
		Expect(testEnv.Client.Create(ctx, cluster)).To(Succeed())

		rcp = &controlplanev1.RKE2ControlPlane{
			Status: controlplanev1.RKE2ControlPlaneStatus{
				Initialized: true,
			},
		}

		cp, err = rke2.NewControlPlane(ctx, testEnv.GetClient(), cluster, rcp, collections.FromMachineList(&ml))
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
		Expect(conditions.IsTrue(machine, controlplanev1.NodeMetadataUpToDate)).To(BeTrue())
		Expect(conditions.IsTrue(machineWithRef, controlplanev1.NodeMetadataUpToDate)).To(BeTrue())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), node)).To(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(nodeByRef), nodeByRef)).To(Succeed())
		Expect(node.GetAnnotations()).To(HaveKeyWithValue("test", "true"))
		Expect(nodeByRef.GetAnnotations()).To(HaveKeyWithValue("test", "true"))
		Expect(conditions.IsFalse(rcp, controlplanev1.ControlPlaneComponentsHealthyCondition)).To(BeTrue())
		Expect(conditions.GetMessage(rcp, controlplanev1.ControlPlaneComponentsHealthyCondition)).To(Equal(
			"Control plane node missing-machine does not have a corresponding machine"))
	})

	It("should rotate kubeconfig secret if needed", func() {
		r := &RKE2ControlPlaneReconciler{
			Client:                    testEnv.GetClient(),
			Scheme:                    testEnv.GetScheme(),
			managementCluster:         &rke2.Management{Client: testEnv.GetClient(), SecretCachingClient: testEnv.GetClient()},
			managementClusterUncached: &rke2.Management{Client: testEnv.GetClient()},
		}
		clusterName := client.ObjectKey{Namespace: ns.Name, Name: "test"}
		endpoint := clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}

		// Create kubeconfig secret with short expiry
		shortExpiryDate := time.Now().Add(24 * time.Hour) // 1 day from now
		secret, err := createKubeconfigSecret(ns.Name, shortExpiryDate)
		Expect(err).ToNot(HaveOccurred())
		Expect(testEnv.Create(ctx, secret)).To(Succeed())

		// Check that rotation is needed
		needsRotation, err := kubeconfig.NeedsClientCertRotation(secret, certs.ClientCertificateRenewalDuration)
		Expect(err).ToNot(HaveOccurred())
		Expect(needsRotation).To(BeTrue())

		// Rotate kubeconfig secret
		_, err = r.reconcileKubeconfig(ctx, clusterName, endpoint, rcp)
		Expect(err).ToNot(HaveOccurred())

		Expect(testEnv.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: secret.Name}, secret)).To(Succeed())
	})

	It("should not rotate kubeconfig secret if not needed", func() {
		r := &RKE2ControlPlaneReconciler{
			Client:                    testEnv.GetClient(),
			Scheme:                    testEnv.GetScheme(),
			managementCluster:         &rke2.Management{Client: testEnv.GetClient(), SecretCachingClient: testEnv.GetClient()},
			managementClusterUncached: &rke2.Management{Client: testEnv.GetClient()},
		}
		clusterName := client.ObjectKey{Namespace: ns.Name, Name: "test"}
		endpoint := clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}

		// Create kubeconfig secret with long expiry
		longExpiryDate := time.Now().Add(365 * 24 * time.Hour) // 1 year from now
		secret, err := createKubeconfigSecret(ns.Name, longExpiryDate)
		Expect(err).ToNot(HaveOccurred())
		Expect(testEnv.Create(ctx, secret)).To(Succeed())

		// Check that no rotation is needed
		needsRotation, err := kubeconfig.NeedsClientCertRotation(secret, certs.ClientCertificateRenewalDuration)
		Expect(err).ToNot(HaveOccurred())
		Expect(needsRotation).To(BeFalse())

		// Ensure no rotation occurs
		_, err = r.reconcileKubeconfig(ctx, clusterName, endpoint, rcp)
		Expect(err).ToNot(HaveOccurred())

		Expect(testEnv.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: secret.Name}, secret)).To(Succeed())
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

// createKubeconfigSecret creates a Kubernetes secret with a kubeconfig containing a client certificate and key.
func createKubeconfigSecret(namespace string, expiryDate time.Time) (*corev1.Secret, error) {
	certPEM, keyPEM, err := generateCertAndKey(expiryDate)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kubeconfig-secret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"value": []byte(fmt.Sprintf(`
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
`, base64.StdEncoding.EncodeToString(certPEM), base64.StdEncoding.EncodeToString(keyPEM))),
		},
	}

	return secret, nil
}
