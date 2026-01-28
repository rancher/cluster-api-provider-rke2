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
	"errors"
	"fmt"

	errorsPkg "github.com/pkg/errors"
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

// rke2ControlPlaneTemplateLogger is the RKE2ControlPlaneTemplate webhook logger.
var rke2ControlPlaneTemplateLogger = logf.Log.WithName("RKE2ControlPlaneTemplate")

// RKE2ControlPlaneTemplateCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind RKE2ControlPlaneTemplate when those are created or updated.
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type RKE2ControlPlaneTemplateCustomDefaulter struct{}

// RKE2ControlPlaneTemplateCustomValidator struct is responsible for validating the RKE2ControlPlaneTemplate resource
// when it is created, updated, or deleted.
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type RKE2ControlPlaneTemplateCustomValidator struct{}

// SetupRKE2ControlPlaneTemplateWebhookWithManager sets up the Controller Manager for the Webhook for the RKE2ControlPlaneTemplate resource.
func SetupRKE2ControlPlaneTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&RKE2ControlPlaneTemplate{}).
		WithValidator(&RKE2ControlPlaneTemplateCustomValidator{}).
		WithDefaulter(&RKE2ControlPlaneTemplateCustomDefaulter{}, admission.DefaulterRemoveUnknownOrOmitableFields).
		Complete()
}

var _ webhook.CustomDefaulter = &RKE2ControlPlaneTemplateCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplateCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	rcpt, ok := obj.(*RKE2ControlPlaneTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a RKE2ControlPlaneTemplate but got a %T", obj))
	}

	rke2ControlPlaneTemplateLogger.Info("defaulting", "RKE2ControlPlaneTemplate", klog.KObj(rcpt))

	bootstrapv1.DefaultRKE2ConfigSpec(&rcpt.Spec.Template.Spec.RKE2ConfigSpec)

	// Correct the additional user data by making it YAML compliant if provided
	if rcpt.Spec.Template.Spec.AgentConfig.AdditionalUserData.Data != nil {
		if err := bootstrapv1.CorrectArbitraryData(rcpt.Spec.Template.Spec.AgentConfig.AdditionalUserData.Data); err != nil {
			return errorsPkg.Wrapf(err, "failed to correct additional user data for RKE2ControlPlaneTemplate")
		}
	}

	return nil
}

var _ webhook.CustomValidator = &RKE2ControlPlaneTemplateCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplateCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rcpt, ok := obj.(*RKE2ControlPlaneTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ControlPlaneTemplate object but got %T", obj)
	}

	rke2ControlPlaneTemplateLogger.Info("validate create", "RKE2ControlPlaneTemplate", klog.KObj(rcpt))

	var allErrs field.ErrorList

	allErrs = append(allErrs, bootstrapv1.ValidateRKE2ConfigSpec(rcpt.Name, &rcpt.Spec.Template.Spec.RKE2ConfigSpec)...)
	allErrs = append(allErrs, rcpt.validateCNI()...)
	allErrs = append(allErrs, rcpt.validateRegistrationMethod()...)

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), rcpt.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplateCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldControlplane, ok := oldObj.(*RKE2ControlPlaneTemplate)
	if !ok {
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), "", field.ErrorList{
			field.InternalError(nil, errors.New("failed to convert old RKE2ControlPlane to object")),
		})
	}

	newControlplane, ok := newObj.(*RKE2ControlPlaneTemplate)
	if !ok {
		// At this point, oldControlplane is guaranteed to be valid
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), oldControlplane.Name, field.ErrorList{
			field.InternalError(nil, errors.New("failed to convert new RKE2ControlPlane to object")),
		})
	}

	rke2ControlPlaneTemplateLogger.Info("validate update", "RKE2ControlPlaneTemplate", klog.KObj(oldControlplane))

	var allErrs field.ErrorList

	allErrs = append(allErrs, bootstrapv1.ValidateRKE2ConfigSpec(newControlplane.Name, &newControlplane.Spec.Template.Spec.RKE2ConfigSpec)...)
	allErrs = append(allErrs, newControlplane.validateCNI()...)

	oldSet := oldControlplane.Spec.Template.Spec.RegistrationMethod != ""
	if oldSet && newControlplane.Spec.Template.Spec.RegistrationMethod != oldControlplane.Spec.Template.Spec.RegistrationMethod {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "registrationMethod"),
				newControlplane.Spec.Template.Spec.RegistrationMethod,
				"field value is immutable once set",
			),
		)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), newControlplane.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplateCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rcpt, ok := obj.(*RKE2ControlPlaneTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ControlPlaneTemplate object but got %T", obj)
	}

	rke2ControlPlaneTemplateLogger.Info("validate delete", "RKE2ControlPlaneTemplate", klog.KObj(rcpt))

	return nil, nil
}

func (rcpt *RKE2ControlPlaneTemplate) validateCNI() field.ErrorList {
	var allErrs field.ErrorList

	spec := rcpt.Spec.Template.Spec

	if spec.ServerConfig.CNIMultusEnable && rcpt.Spec.Template.Spec.ServerConfig.CNI == "" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "serverConfig", "cni"),
				rcpt.Spec.Template.Spec.ServerConfig.CNI, "must be specified when cniMultusEnable is true"))
	}

	return allErrs
}

func (rcpt *RKE2ControlPlaneTemplate) validateRegistrationMethod() field.ErrorList {
	var allErrs field.ErrorList

	spec := rcpt.Spec.Template.Spec

	if spec.RegistrationMethod == RegistrationMethodAddress {
		if spec.RegistrationAddress == "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec.registrationAddress"),
					spec.RegistrationAddress, "registrationAddress must be supplied when using registration method 'address'"))
		}
	}

	return allErrs
}
