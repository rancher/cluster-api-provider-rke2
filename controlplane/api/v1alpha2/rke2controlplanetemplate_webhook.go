/*
Copyright 2022 SUSE.

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

package v1alpha2

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var rke2controlplanetemplatelog = logf.Log.WithName("rke2controlplanetemplate-resource")

// SetupWebhookWithManager sets up the Controller Manager for the Webhook for the RKE2ControlPlaneTemplate resource.
func (r *RKE2ControlPlaneTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-controlplane-cluster-x-k8s-io-v1alpha2-rke2controlplanetemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanetemplates,verbs=create;update,versions=v1alpha2,name=mrke2controlplanetemplate.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &RKE2ControlPlaneTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplate) Default() {
	rke2controlplanetemplatelog.Info("default", "name", r.Name)
}

//+kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1alpha2-rke2controlplanetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanetemplates,verbs=create;update,versions=v1alpha2,name=vrke2controlplanetemplate.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &RKE2ControlPlaneTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplate) ValidateCreate() (admission.Warnings, error) {
	rke2controlplanetemplatelog.Info("validate create", "name", r.Name)

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplate) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	rke2controlplanetemplatelog.Info("validate update", "name", r.Name)

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplate) ValidateDelete() (admission.Warnings, error) {
	rke2controlplanetemplatelog.Info("validate delete", "name", r.Name)

	return nil, nil
}
