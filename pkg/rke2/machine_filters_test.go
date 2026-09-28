package rke2

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/collections"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
)

var (
	k8sMachineVersion  = "v1.24.6"
	rke2MachineVersion = "v1.24.6+rke2r1"
	regionEuCentral1   = "eu-central-1"
)

var rcp = controlplanev1.RKE2ControlPlane{
	ObjectMeta: v1.ObjectMeta{
		Name:      "rke2-cluster-control-plane",
		Namespace: "example",
	},
	Spec: controlplanev1.RKE2ControlPlaneSpec{
		Version: rke2MachineVersion,
		ServerConfig: controlplanev1.RKE2ServerConfig{
			CNI:               "calico",
			CloudProviderName: "aws",
			ClusterDomain:     "example.com",
		},
		RKE2ConfigSpec: bootstrapv1.RKE2ConfigSpec{
			AgentConfig: bootstrapv1.RKE2AgentConfig{
				NodeLabels: []string{"hello=world"},
			},
		},
	},
}

var machine = clusterv1.Machine{
	ObjectMeta: v1.ObjectMeta{
		Name:      "machine-test",
		Namespace: "example",
		Annotations: map[string]string{
			controlplanev1.RKE2ServerConfigurationAnnotation: "{\"cni\":\"calico\",\"cloudProviderName\":\"aws\",\"clusterDomain\":\"example.com\"}",
		},
	},
	Spec: clusterv1.MachineSpec{
		ClusterName:   "rke2-cluster",
		Version:       k8sMachineVersion,
		FailureDomain: regionEuCentral1,
		Bootstrap: clusterv1.Bootstrap{
			ConfigRef: clusterv1.ContractVersionedObjectReference{
				Kind: "RKE2ConfigTemplate",
				Name: "rke2-cluster-config-template",
			},
		},
	},
}

var _ = Describe("ServerConfigMatching", func() {
	It("should match the machine annotation", func() {
		res := matchServerConfig(context.TODO(), &rcp, &machine)
		Expect(res).To(BeTrue())
	})
})

var _ = Describe("matchAgentConfig", func() {
	It("should match Agent Config", func() {
		machineConfigs := map[string]*bootstrapv1.RKE2Config{
			"someMachine": {},
			"machine-test": {
				ObjectMeta: v1.ObjectMeta{
					Name:      "rke2-config-example",
					Namespace: "example",
				},
				Spec: bootstrapv1.RKE2ConfigSpec{
					AgentConfig: bootstrapv1.RKE2AgentConfig{
						NodeLabels: []string{"hello=world"},
					},
				},
			},
		}
		machineCollection := collections.FromMachines(&machine)
		Expect(len(machineCollection)).To(Equal(1))
		matches := machineCollection.AnyFilter(matchesRKE2BootstrapConfig(context.TODO(), machineConfigs, &rcp))

		Expect(len(matches)).To(Equal(1))
		Expect(matches.Oldest().Name).To(Equal("machine-test"))
	},
	)

	It("shouldn't match Agent Config and different preBootstrapCommands", func() {
		machineConfigs := map[string]*bootstrapv1.RKE2Config{
			"someMachine": {},
			"machine-test": {
				ObjectMeta: v1.ObjectMeta{
					Name:      "rke2-config-example",
					Namespace: "example",
				},
				Spec: bootstrapv1.RKE2ConfigSpec{
					AgentConfig: bootstrapv1.RKE2AgentConfig{
						NodeLabels: []string{"hello=world"},
					},
					PreRKE2Commands: []string{"test"},
				},
			},
		}
		machineCollection := collections.FromMachines(&machine)
		Expect(len(machineCollection)).To(Equal(1))
		matches := machineCollection.AnyFilter(matchesRKE2BootstrapConfig(context.TODO(), machineConfigs, &rcp))

		Expect(len(matches)).To(Equal(0))
	},
	)

	It("shouldn't match Agent Config and different postBootstrapCommands", func() {
		machineConfigs := map[string]*bootstrapv1.RKE2Config{
			"someMachine": {},
			"machine-test": {
				ObjectMeta: v1.ObjectMeta{
					Name:      "rke2-config-example",
					Namespace: "example",
				},
				Spec: bootstrapv1.RKE2ConfigSpec{
					AgentConfig: bootstrapv1.RKE2AgentConfig{
						NodeLabels: []string{"hello=world"},
					},
					PostRKE2Commands: []string{"test"},
				},
			},
		}
		machineCollection := collections.FromMachines(&machine)
		Expect(len(machineCollection)).To(Equal(1))
		matches := machineCollection.AnyFilter(matchesRKE2BootstrapConfig(context.TODO(), machineConfigs, &rcp))

		Expect(len(matches)).To(Equal(0))
	},
	)
})

var _ = Describe("matching Kubernetes Version", func() {
	It("should match version", func() {
		machineCollection := collections.FromMachines(&machine)
		matches := machineCollection.AnyFilter(matchesKubernetesOrRKE2Version(context.TODO(), rcp.GetDesiredVersion()))
		Expect(len(matches)).To(Equal(1))
	})

	It("should match when RKE2 version is set on the machine", func() {
		machine.Spec.Version = rke2MachineVersion
		machineCollection := collections.FromMachines(&machine)
		matches := machineCollection.AnyFilter(matchesKubernetesOrRKE2Version(context.TODO(), rcp.GetDesiredVersion()))
		Expect(len(matches)).To(Equal(1))
		machine.Spec.Version = k8sMachineVersion
	})
})

func TestMachineTaintsRollout(t *testing.T) {
	taint := clusterv1.MachineTaint{
		Key:         "workload.example.com/dedicated",
		Value:       "control-plane",
		Effect:      corev1.TaintEffectPreferNoSchedule,
		Propagation: clusterv1.MachineTaintPropagationAlways,
	}
	updatedTaint := taint
	updatedTaint.Value = "updated"
	updatedTaint.Propagation = clusterv1.MachineTaintPropagationOnInitialization

	tests := []struct {
		name                   string
		currentTaints          []clusterv1.MachineTaint
		desiredTaints          []clusterv1.MachineTaint
		currentBootstrapTaints []string
		desiredBootstrapTaints []string
		upToDate               bool
	}{
		{
			name:          "adding machine template taints does not trigger rollout",
			desiredTaints: []clusterv1.MachineTaint{taint},
			upToDate:      true,
		},
		{
			name:          "updating machine template taints does not trigger rollout",
			currentTaints: []clusterv1.MachineTaint{taint},
			desiredTaints: []clusterv1.MachineTaint{updatedTaint},
			upToDate:      true,
		},
		{
			name:          "removing machine template taints does not trigger rollout",
			currentTaints: []clusterv1.MachineTaint{taint},
			upToDate:      true,
		},
		{
			name:                   "adding bootstrap node taints still triggers rollout",
			desiredBootstrapTaints: []string{"workload.example.com/dedicated=control-plane:PreferNoSchedule"},
		},
		{
			name:                   "updating bootstrap node taints still triggers rollout",
			currentBootstrapTaints: []string{"workload.example.com/dedicated=control-plane:PreferNoSchedule"},
			desiredBootstrapTaints: []string{"workload.example.com/dedicated=updated:PreferNoSchedule"},
		},
		{
			name:                   "removing bootstrap node taints still triggers rollout",
			currentBootstrapTaints: []string{"workload.example.com/dedicated=control-plane:PreferNoSchedule"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			controlPlane := rcp.DeepCopy()
			controlPlane.Spec.MachineTemplate.Spec.Taints = tt.desiredTaints
			controlPlane.Spec.RKE2ConfigSpec.AgentConfig.NodeTaints = tt.desiredBootstrapTaints
			currentMachine := machine.DeepCopy()
			currentMachine.Spec.Version = controlPlane.Spec.Version
			currentMachine.Spec.Taints = tt.currentTaints
			machineConfig := &bootstrapv1.RKE2Config{Spec: *controlPlane.Spec.RKE2ConfigSpec.DeepCopy()}
			machineConfig.Spec.AgentConfig.NodeTaints = tt.currentBootstrapTaints
			machineConfigs := map[string]*bootstrapv1.RKE2Config{currentMachine.Name: machineConfig}
			cluster := &clusterv1.Cluster{ObjectMeta: v1.ObjectMeta{Name: currentMachine.Spec.ClusterName}}

			upToDate, result, err := UpToDate(t.Context(), nil, cluster, currentMachine, controlPlane, nil, machineConfigs)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(upToDate).To(Equal(tt.upToDate))
			g.Expect(result.DesiredMachine.Spec.Taints).To(Equal(tt.desiredTaints))
			if tt.upToDate {
				g.Expect(result.ConditionMessages).To(BeEmpty())
				g.Expect(result.EligibleForInPlaceUpdate).To(BeFalse())
			} else {
				g.Expect(result.ConditionMessages).To(ConsistOf("RKE2Config is not up-to-date"))
			}
		})
	}
}
