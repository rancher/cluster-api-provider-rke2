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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// RKE2configtemplatelog is for logging in this package.
var RKE2configtemplatelog = logf.Log.WithName("RKE2configtemplate-resource")

// RKE2ConfigTemplateCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind RKE2ConfigTemplate when those are created or updated.
type RKE2ConfigTemplateCustomDefaulter struct{}

// RKE2ConfigTemplateCustomValidator struct is responsible for validating the RKE2ConfigTemplate resource
// when it is created, updated, or deleted.
type RKE2ConfigTemplateCustomValidator struct{}

// SetupRKE2ConfigTemplateWebhookWithManager sets up the Controller Manager for the Webhook for the RKE2ControlPlaneTemplate resource.
func SetupRKE2ConfigTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&RKE2ConfigTemplate{}).
		WithValidator(&RKE2ConfigTemplateCustomValidator{}).
		WithDefaulter(&RKE2ConfigTemplateCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-bootstrap-cluster-x-k8s-io-v1beta1-rke2configtemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=bootstrap.cluster.x-k8s.io,resources=rke2configtemplates,verbs=create;update,versions=v1beta1,name=mrke2configtemplate.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &RKE2ConfigTemplateCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *RKE2ConfigTemplateCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	rct, ok := obj.(*RKE2ConfigTemplate)
	if !ok {
		return fmt.Errorf("expected a RKE2ConfigTemplate object but got %T", obj)
	}

	RKE2configtemplatelog.Info("default", "name", rct.Name)

	return nil
}

//+kubebuilder:webhook:path=/validate-bootstrap-cluster-x-k8s-io-v1beta1-rke2configtemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=bootstrap.cluster.x-k8s.io,resources=rke2configtemplates,verbs=create;update,versions=v1beta1,name=vrke2configtemplate.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &RKE2ConfigTemplateCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ConfigTemplateCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rct, ok := obj.(*RKE2ConfigTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ConfigTemplate object but got %T", obj)
	}

	RKE2configtemplatelog.Info("validate create", "name", rct.Name)

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ConfigTemplateCustomValidator) ValidateUpdate(_ context.Context, oldObj, _ runtime.Object) (admission.Warnings, error) {
	rct, ok := oldObj.(*RKE2ConfigTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ConfigTemplate object but got %T", oldObj)
	}

	RKE2configtemplatelog.Info("validate update", "name", rct.Name)

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ConfigTemplateCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rct, ok := obj.(*RKE2ConfigTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2ConfigTemplate object but got %T", obj)
	}

	RKE2configtemplatelog.Info("validate delete", "name", rct.Name)

	return nil, nil
}
