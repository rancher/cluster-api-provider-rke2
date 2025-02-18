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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
)

// RKE2ControlPlaneTemplateCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind RKE2ControlPlaneTemplate when those are created or updated.
type RKE2ControlPlaneTemplateCustomDefaulter struct{}

// RKE2ControlPlaneTemplateCustomValidator struct is responsible for validating the RKE2ControlPlaneTemplate resource
// when it is created, updated, or deleted.
type RKE2ControlPlaneTemplateCustomValidator struct{}

// SetupRKE2ControlPlaneTemplateWebhookWithManager sets up the Controller Manager for the Webhook for the RKE2ControlPlaneTemplate resource.
func SetupRKE2ControlPlaneTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&RKE2ControlPlaneTemplate{}).
		WithValidator(&RKE2ControlPlaneTemplateCustomValidator{}).
		WithDefaulter(&RKE2ControlPlaneTemplateCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-controlplane-cluster-x-k8s-io-v1beta1-rke2controlplanetemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanetemplates,verbs=create;update,versions=v1beta1,name=mrke2controlplanetemplate.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &RKE2ControlPlaneTemplateCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplateCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	rcpt, ok := obj.(*RKE2ControlPlaneTemplate)
	if !ok {
		return fmt.Errorf("expected a RKE2ControlPlaneTemplate object but got %T", obj)
	}

	bootstrapv1.DefaultRKE2ConfigSpec(&rcpt.Spec.Template.Spec.RKE2ConfigSpec)

	return nil
}

//+kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1beta1-rke2controlplanetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanetemplates,verbs=create;update,versions=v1beta1,name=vrke2controlplanetemplate.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &RKE2ControlPlaneTemplateCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplateCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rcpt, ok := obj.(*RKE2ControlPlaneTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ControlPlaneTemplate object but got %T", obj)
	}

	rke2controlplanelog.Info("RKE2ControlPlane validate create", "control-plane", klog.KObj(rcpt))

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
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), oldControlplane.Name, field.ErrorList{
			field.InternalError(nil, errors.New("failed to convert old RKE2ControlPlane to object")),
		})
	}

	newControlplane, ok := newObj.(*RKE2ControlPlaneTemplate)
	if !ok {
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), oldControlplane.Name, field.ErrorList{
			field.InternalError(nil, errors.New("failed to convert new RKE2ControlPlane to object")),
		})
	}

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

	rke2controlplanelog.Info("validate delete", "name", rcpt.Name)

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
