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

package v1beta2

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
)

const (
	defaultNodeDeletionTimeoutSeconds     = 10
	defaultNodeDrainTimeoutSeconds        = 120
	defaultNodeVolumeDetachTimeoutSeconds = 300
)

// rke2ControlPlaneLogger is the RKE2ControlPlane webhook logger.
var rke2ControlPlaneLogger = logf.Log.WithName("RKE2ControlPlane")

// RKE2ControlPlaneCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind RKE2ControlPlane when those are created or updated.
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type RKE2ControlPlaneCustomDefaulter struct{}

// RKE2ControlPlaneCustomValidator struct is responsible for validating the RKE2ControlPlane resource
// when it is created, updated, or deleted.
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type RKE2ControlPlaneCustomValidator struct{}

// SetupRKE2ControlPlaneWebhookWithManager sets up the Controller Manager for the Webhook for the RKE2ControlPlaneTemplate resource.
func SetupRKE2ControlPlaneWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&RKE2ControlPlane{}).
		WithValidator(&RKE2ControlPlaneCustomValidator{}).
		WithDefaulter(&RKE2ControlPlaneCustomDefaulter{}, admission.DefaulterRemoveUnknownOrOmitableFields).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-controlplane-cluster-x-k8s-io-v1beta2-rke2controlplane,mutating=true,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=create;update,versions=v1beta2,name=mrke2controlplane.kb.io,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomDefaulter = &RKE2ControlPlaneCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (rd *RKE2ControlPlaneCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	rcp, ok := obj.(*RKE2ControlPlane)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a RKE2ControlPlane but got a %T", obj))
	}

	rke2ControlPlaneLogger.Info("defaulting", "RKE2ControlPlane", klog.KObj(rcp))

	bootstrapv1.DefaultRKE2ConfigSpec(&rcp.Spec.RKE2ConfigSpec)

	// Set default NodeDrainTimeout if not set
	if rcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds == nil {
		rcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds = ptr.To(int32(defaultNodeDrainTimeoutSeconds))
	}

	// Set default NodeVolumeDetachTimeout if not set
	if rcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds == nil {
		rcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = ptr.To(int32(defaultNodeVolumeDetachTimeoutSeconds))
	}

	// Set default NodeDeletionTimeout if not set
	if rcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds == nil {
		rcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds = ptr.To(int32(defaultNodeDeletionTimeoutSeconds))
	}

	// Set replicas to 1 if not set
	if rcp.Spec.Replicas == nil {
		replicas := int32(1)
		rcp.Spec.Replicas = &replicas
	}

	// Correct the additional user data by making it YAML compliant if provided
	if rcp.Spec.AgentConfig.AdditionalUserData.Data == nil {
		if err := bootstrapv1.CorrectArbitraryData(rcp.Spec.AgentConfig.AdditionalUserData.Data); err != nil {
			return errors.Wrap(err, "failed to correct additional user data for RKE2ControlPlane")
		}
	}

	return nil
}

//+kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1beta2-rke2controlplane,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=create;update,versions=v1beta2,name=vrke2controlplane.kb.io,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomValidator = &RKE2ControlPlaneCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (rv *RKE2ControlPlaneCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rcp, ok := obj.(*RKE2ControlPlane)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ControlPlane object but got %T", obj)
	}

	rke2ControlPlaneLogger.Info("validate create", "RKE2ControlPlane", klog.KObj(rcp))

	var allErrs field.ErrorList

	allErrs = append(allErrs, bootstrapv1.ValidateRKE2ConfigSpec(rcp.Name, &rcp.Spec.RKE2ConfigSpec)...)
	allErrs = append(allErrs, rcp.validateCNI()...)
	allErrs = append(allErrs, rcp.validateRegistrationMethod()...)
	allErrs = append(allErrs, rcp.validateMachineTemplate()...)
	allErrs = append(allErrs, rcp.validateSpec()...)

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), rcp.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (rv *RKE2ControlPlaneCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldControlplane, ok := oldObj.(*RKE2ControlPlane)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ControlPlane object but got %T", oldObj)
	}

	newControlplane, ok := newObj.(*RKE2ControlPlane)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ControlPlane object but got %T", newObj)
	}

	rke2ControlPlaneLogger.Info("validate update", "RKE2ControlPlane", klog.KObj(oldControlplane))

	var allErrs field.ErrorList

	allErrs = append(allErrs, bootstrapv1.ValidateRKE2ConfigSpec(newControlplane.Name, &newControlplane.Spec.RKE2ConfigSpec)...)
	allErrs = append(allErrs, newControlplane.validateCNI()...)
	allErrs = append(allErrs, newControlplane.validateMachineTemplate()...)
	allErrs = append(allErrs, newControlplane.validateSpec()...)

	oldSet := oldControlplane.Spec.RegistrationMethod != ""
	if oldSet && newControlplane.Spec.RegistrationMethod != oldControlplane.Spec.RegistrationMethod {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "registrationMethod"), newControlplane.Spec.RegistrationMethod, "field value is immutable once set"),
		)
	}

	// Ensure NodeDrainTimeoutSeconds is mutable.
	oldNodeDrainTimeout := oldControlplane.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds
	newNodeDrainTimeout := newControlplane.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds

	if oldNodeDrainTimeout != nil && newNodeDrainTimeout != nil && !ptr.Equal(oldNodeDrainTimeout, newNodeDrainTimeout) {
		rke2ControlPlaneLogger.Info(
			"NodeDrainTimeoutSeconds field updated",
			"old", oldNodeDrainTimeout,
			"new", newNodeDrainTimeout,
		)
	}

	// Ensure NodeVolumeDetachTimeoutSeconds is mutable.
	oldNodeVolumeDetachTimeout := oldControlplane.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds
	newNodeVolumeDetachTimeout := newControlplane.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds

	if oldNodeVolumeDetachTimeout != nil && newNodeVolumeDetachTimeout != nil && !ptr.Equal(oldNodeVolumeDetachTimeout, newNodeVolumeDetachTimeout) {
		rke2ControlPlaneLogger.Info(
			"NodeVolumeDetachTimeoutSeconds field updated",
			"old", oldNodeVolumeDetachTimeout,
			"new", newNodeVolumeDetachTimeout,
		)
	}

	// Ensure NodeDeletionTimeoutSeconds is mutable.
	oldNodeDeletionTimeout := oldControlplane.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds
	newNodeDeletionTimeout := newControlplane.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds

	if oldNodeDeletionTimeout != nil && newNodeDeletionTimeout != nil && !ptr.Equal(oldNodeDeletionTimeout, newNodeDeletionTimeout) {
		rke2ControlPlaneLogger.Info(
			"NodeDeletionTimeoutSeconds field updated",
			"old", oldNodeDeletionTimeout,
			"new", newNodeDeletionTimeout,
		)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), newControlplane.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (rv *RKE2ControlPlaneCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rcp, ok := obj.(*RKE2ControlPlane)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ControlPlane object but got %T", obj)
	}

	rke2ControlPlaneLogger.Info("validate delete", "RKE2ControlPlane", klog.KObj(rcp))

	return nil, nil
}

func (r *RKE2ControlPlane) validateCNI() field.ErrorList {
	var allErrs field.ErrorList

	if r.Spec.ServerConfig.CNIMultusEnable && r.Spec.ServerConfig.CNI == "" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "serverConfig", "cni"),
				r.Spec.ServerConfig.CNI, "must be specified when cniMultusEnable is true"))
	}

	return allErrs
}

func (r *RKE2ControlPlane) validateRegistrationMethod() field.ErrorList {
	var allErrs field.ErrorList

	if r.Spec.RegistrationMethod == RegistrationMethodAddress {
		if r.Spec.RegistrationAddress == "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec.registrationAddress"),
					r.Spec.RegistrationAddress, "registrationAddress must be supplied when using registration method 'address'"))
		}
	}

	return allErrs
}

func (r *RKE2ControlPlane) validateMachineTemplate() field.ErrorList {
	var allErrs field.ErrorList

	// Validate InfrastructureRef is set.
	if !r.Spec.MachineTemplate.Spec.InfrastructureRef.IsDefined() {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "machineTemplate", "spec", "infrastructureRef"),
				r.Spec.MachineTemplate.Spec.InfrastructureRef, "machineTemplate is required"))
	}

	// Validate NodeDrainTimeoutSeconds (must be non-negative).
	nodeDrainTimeout := r.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds
	if nodeDrainTimeout != nil && *nodeDrainTimeout < 0 {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "machineTemplate", "spec", "deletion", "nodeDrainTimeoutSeconds"),
				nodeDrainTimeout, "must be non-negative"))
	}

	// Validate NodeVolumeDetachTimeoutSeconds (must be non-negative).
	nodeVolumeDetachTimeout := r.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds
	if nodeVolumeDetachTimeout != nil && *nodeVolumeDetachTimeout < 0 {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "machineTemplate", "spec", "deletion", "nodeVolumeDetachTimeoutSeconds"),
				nodeVolumeDetachTimeout, "must be non-negative"))
	}

	// Validate NodeDeletionTimeoutSeconds (must be non-negative).
	nodeDeletionTimeout := r.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds
	if nodeDeletionTimeout != nil && *nodeDeletionTimeout < 0 {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "machineTemplate", "spec", "deletion", "nodeDeletionTimeoutSeconds"),
				nodeDeletionTimeout, "must be non-negative"))
	}

	return allErrs
}

func (r *RKE2ControlPlane) validateSpec() field.ErrorList {
	var allErrs field.ErrorList

	if r.Spec.Replicas == nil {
		allErrs = append(
			allErrs,
			field.Required(
				field.NewPath("spec", "replicas"),
				"is required",
			),
		)
	} else if *r.Spec.Replicas <= 0 {
		allErrs = append(
			allErrs,
			field.Forbidden(
				field.NewPath("spec", "replicas"),
				"cannot be less than or equal to 0",
			),
		)
	}

	return allErrs
}
