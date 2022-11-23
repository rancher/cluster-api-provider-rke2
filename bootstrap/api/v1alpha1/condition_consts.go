package v1alpha1

import (
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	DataSecretAvailableCondition clusterv1.ConditionType = "Available"
)

const (
	WaitingForClusterInfrastructureReason string = "WaitingForClusterInfrastructure"
)
