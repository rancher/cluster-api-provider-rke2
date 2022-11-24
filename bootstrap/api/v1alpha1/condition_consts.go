package v1alpha1

import (
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// DataSecretAvailableCondition documents the status of the bootstrap secret generation process.
	//
	// NOTE: When the DataSecret generation starts the process completes immediately and within the
	// same reconciliation, so the user will always see a transition from Wait to Generated without having
	DataSecretAvailableCondition clusterv1.ConditionType = "Available"
)

const (
	// WaitingForClusterInfrastructureReason (Severity=Info) document a bootstrap secret generation process
	// waiting for the cluster infrastructure to be ready.
	//
	// NOTE: Having the cluster infrastructure ready is a pre-condition for starting to create machines;

	WaitingForClusterInfrastructureReason string = "WaitingForClusterInfrastructure"
)
