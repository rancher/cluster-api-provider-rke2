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
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// rke2ConfigTemplateLogger the RKE2ConfigTemplate webhook logger.
var rke2ConfigTemplateLogger = logf.Log.WithName("RKE2ConfigTemplate")

// RKE2ConfigTemplateCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind RKE2ConfigTemplate when those are created or updated.
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type RKE2ConfigTemplateCustomDefaulter struct{}

// RKE2ConfigTemplateCustomValidator struct is responsible for validating the RKE2ConfigTemplate resource
// when it is created, updated, or deleted.
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type RKE2ConfigTemplateCustomValidator struct{}

// SetupRKE2ConfigTemplateWebhookWithManager sets up the Controller Manager for the Webhook for the RKE2ControlPlaneTemplate resource.
func SetupRKE2ConfigTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&RKE2ConfigTemplate{}).
		WithValidator(&RKE2ConfigTemplateCustomValidator{}).
		WithDefaulter(&RKE2ConfigTemplateCustomDefaulter{}, admission.DefaulterRemoveUnknownOrOmitableFields).
		Complete()
}

var _ webhook.CustomDefaulter = &RKE2ConfigTemplateCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *RKE2ConfigTemplateCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	rct, ok := obj.(*RKE2ConfigTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a RKE2ConfigTemplate but got a %T", obj))
	}

	rke2ConfigTemplateLogger.Info("defaulting", "RKE2ConfigTemplate", klog.KObj(rct))

	DefaultRKE2ConfigSpec(&rct.Spec.Template.Spec)

	return nil
}

var _ webhook.CustomValidator = &RKE2ConfigTemplateCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ConfigTemplateCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rct, ok := obj.(*RKE2ConfigTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ConfigTemplate object but got %T", obj)
	}

	rke2ConfigTemplateLogger.Info("validate create", "RKE2ConfigTemplate", klog.KObj(rct))

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ConfigTemplateCustomValidator) ValidateUpdate(_ context.Context, oldObj, _ runtime.Object) (admission.Warnings, error) {
	rct, ok := oldObj.(*RKE2ConfigTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ConfigTemplate object but got %T", oldObj)
	}

	rke2ConfigTemplateLogger.Info("validate update", "RKE2ConfigTemplate", klog.KObj(rct))

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ConfigTemplateCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rct, ok := obj.(*RKE2ConfigTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ConfigTemplate object but got %T", obj)
	}

	rke2ConfigTemplateLogger.Info("validate delete", "RKE2ConfigTemplate", klog.KObj(rct))

	return nil, nil
}
