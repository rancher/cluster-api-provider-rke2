/*
Copyright 2026 SUSE LLC.

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

package v1beta2

import (
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// RKE2Config's Ready Condition and Reasons.
const (
	// RKE2ConfigReadyCondition is true if the RKE2Config is not deleted,
	// and both DataSecretCreated, CertificatesAvailable conditions are true.
	RKE2ConfigReadyCondition = clusterv1.ReadyCondition

	// RKE2ConfigReadyReason surfaces when the RKE2Config is ready.
	RKE2ConfigReadyReason = clusterv1.ReadyReason

	// RKE2ConfigNotReadyReason surfaces when the RKE2Config is not ready.
	RKE2ConfigNotReadyReason = clusterv1.NotReadyReason

	// RKE2ConfigReadyUnknownReason surfaces when RKE2Config readiness is unknown.
	RKE2ConfigReadyUnknownReason = clusterv1.ReadyUnknownReason
)

// RKE2Config's DataSecretAvailable Condition and Reasons.
const (
	// RKE2ConfigDataSecretAvailableCondition documents the status of the bootstrap secret generation process.
	//
	// NOTE: When the DataSecret generation starts the process completes immediately and within the
	// same reconciliation, so the user will always see a transition from Wait to Generated without having
	// to wait for the next reconciliation.
	RKE2ConfigDataSecretAvailableCondition = "DataSecretAvailable"

	// RKE2ConfigDataSecretAvailableReason surfaces when the bootstrap secret is available.
	RKE2ConfigDataSecretAvailableReason = clusterv1.AvailableReason

	// RKE2ConfigDataSecretNotAvailableReason surfaces when the bootstrap secret is not available.
	RKE2ConfigDataSecretNotAvailableReason = clusterv1.NotAvailableReason
)

const (

	// RKE2ConfigWaitingForClusterInfrastructureReason (Severity=Info) document a bootstrap secret generation process
	// waiting for the cluster infrastructure to be ready.
	//
	// NOTE: Having the cluster infrastructure ready is a pre-condition for starting to create machines.
	RKE2ConfigWaitingForClusterInfrastructureReason string = "WaitingForClusterInfrastructure"
)

const (
	// RKE2ConfigCertificatesAvailableCondition documents the status of the certificates generation process.
	RKE2ConfigCertificatesAvailableCondition = "CertificatesAvailable"

	// RKE2ConfigCertificatesAvailableReason surfaces when certificates required for machine bootstrap are is available.
	RKE2ConfigCertificatesAvailableReason = clusterv1.AvailableReason

	// RKE2ConfigCertificatesAvailableInternalErrorReason surfaces unexpected failures when reading or
	// generating certificates required for machine bootstrap.
	RKE2ConfigCertificatesAvailableInternalErrorReason string = clusterv1.InternalErrorReason
)
