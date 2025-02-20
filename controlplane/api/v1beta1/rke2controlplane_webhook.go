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

package v1beta1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
)

// log is for logging in this package.
var rke2controlplanelog = logf.Log.WithName("rke2controlplane-resource")

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
		WithDefaulter(&RKE2ControlPlaneCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-controlplane-cluster-x-k8s-io-v1beta1-rke2controlplane,mutating=true,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=create;update,versions=v1beta1,name=mrke2controlplane.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &RKE2ControlPlaneCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (rd *RKE2ControlPlaneCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	rcp, ok := obj.(*RKE2ControlPlane)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a RKE2ControlPlane but got a %T", obj))
	}

	bootstrapv1.DefaultRKE2ConfigSpec(&rcp.Spec.RKE2ConfigSpec)

	// Defaults missing MachineTemplate.InfrastructureRef to Spec.InfrastructureRef
	if len(rcp.Spec.MachineTemplate.InfrastructureRef.Name) == 0 {
		rcp.Spec.MachineTemplate.InfrastructureRef = rcp.Spec.InfrastructureRef
	}

	// Defaults missing MachineTemplate.NodeDrainTimeout to Spec.NodeDrainTimeout
	if rcp.Spec.MachineTemplate.NodeDrainTimeout == nil {
		rcp.Spec.MachineTemplate.NodeDrainTimeout = rcp.Spec.NodeDrainTimeout
	}

	return nil
}

//+kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1beta1-rke2controlplane,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=create;update,versions=v1beta1,name=vrke2controlplane.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &RKE2ControlPlaneCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (rv *RKE2ControlPlaneCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rcp, ok := obj.(*RKE2ControlPlane)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ControlPlane object but got %T", obj)
	}

	rke2controlplanelog.Info("RKE2ControlPlane validate create", "control-plane", klog.KObj(rcp))

	var allErrs field.ErrorList

	allErrs = append(allErrs, bootstrapv1.ValidateRKE2ConfigSpec(rcp.Name, &rcp.Spec.RKE2ConfigSpec)...)
	allErrs = append(allErrs, rcp.validateCNI()...)
	allErrs = append(allErrs, rcp.validateRegistrationMethod()...)
	allErrs = append(allErrs, rcp.validateMachineTemplate()...)

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

	var allErrs field.ErrorList

	allErrs = append(allErrs, bootstrapv1.ValidateRKE2ConfigSpec(newControlplane.Name, &newControlplane.Spec.RKE2ConfigSpec)...)
	allErrs = append(allErrs, newControlplane.validateCNI()...)
	allErrs = append(allErrs, newControlplane.validateMachineTemplate()...)

	oldSet := oldControlplane.Spec.RegistrationMethod != ""
	if oldSet && newControlplane.Spec.RegistrationMethod != oldControlplane.Spec.RegistrationMethod {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "registrationMethod"), newControlplane.Spec.RegistrationMethod, "field value is immutable once set"),
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

	rke2controlplanelog.Info("validate delete", "name", rcp.Name)

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

	if r.Spec.MachineTemplate.InfrastructureRef.Name == "" && r.Spec.InfrastructureRef.Name == "" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "machineTemplate", "infrastructureRef"),
				r.Spec.MachineTemplate.InfrastructureRef, "machineTemplate is required"))
	}

	return allErrs
}
