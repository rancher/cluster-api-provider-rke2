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
	"errors"

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

// SetupWebhookWithManager sets up the Controller Manager for the Webhook for the RKE2ControlPlane resource.
func (r *RKE2ControlPlane) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-controlplane-cluster-x-k8s-io-v1beta1-rke2controlplane,mutating=true,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=create;update,versions=v1beta1,name=mrke2controlplane.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &RKE2ControlPlane{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *RKE2ControlPlane) Default() {
	bootstrapv1.DefaultRKE2ConfigSpec(&r.Spec.RKE2ConfigSpec)

	// Defaults missing MachineTemplate.InfrastructureRef to Spec.InfrastructureRef
	if len(r.Spec.MachineTemplate.InfrastructureRef.Name) == 0 {
		r.Spec.MachineTemplate.InfrastructureRef = r.Spec.InfrastructureRef
	}

	// Defaults missing MachineTemplate.NodeDrainTimeout to Spec.NodeDrainTimeout
	if r.Spec.MachineTemplate.NodeDrainTimeout == nil {
		r.Spec.MachineTemplate.NodeDrainTimeout = r.Spec.NodeDrainTimeout
	}
}

//+kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1beta1-rke2controlplane,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=create;update,versions=v1beta1,name=vrke2controlplane.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &RKE2ControlPlane{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlane) ValidateCreate() (admission.Warnings, error) {
	rke2controlplanelog.Info("RKE2ControlPlane validate create", "control-plane", klog.KObj(r))

	var allErrs field.ErrorList

	allErrs = append(allErrs, bootstrapv1.ValidateRKE2ConfigSpec(r.Name, &r.Spec.RKE2ConfigSpec)...)
	allErrs = append(allErrs, r.validateCNI()...)
	allErrs = append(allErrs, r.validateRegistrationMethod()...)
	allErrs = append(allErrs, r.validateMachineTemplate()...)

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlane) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	oldControlplane, ok := old.(*RKE2ControlPlane)
	if !ok {
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), r.Name, field.ErrorList{
			field.InternalError(nil, errors.New("failed to convert old RKE2ControlPlane to object")),
		})
	}

	var allErrs field.ErrorList

	allErrs = append(allErrs, bootstrapv1.ValidateRKE2ConfigSpec(r.Name, &r.Spec.RKE2ConfigSpec)...)
	allErrs = append(allErrs, r.validateCNI()...)
	allErrs = append(allErrs, r.validateMachineTemplate()...)

	oldSet := oldControlplane.Spec.RegistrationMethod != ""
	if oldSet && r.Spec.RegistrationMethod != oldControlplane.Spec.RegistrationMethod {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "registrationMethod"), r.Spec.RegistrationMethod, "field value is immutable once set"),
		)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlane) ValidateDelete() (admission.Warnings, error) {
	rke2controlplanelog.Info("validate delete", "name", r.Name)

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
