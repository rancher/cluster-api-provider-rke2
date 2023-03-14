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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var rke2controlplanelog = logf.Log.WithName("rke2controlplane-resource")

// SetupWebhookWithManager sets up the Controller Manager for the Webhook for the RKE2ControlPlane resource.
func (r *RKE2ControlPlane) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-controlplane-cluster-x-k8s-io-v1alpha1-rke2controlplane,mutating=true,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=create;update,versions=v1alpha1,name=mrke2controlplane.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &RKE2ControlPlane{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *RKE2ControlPlane) Default() {
	rke2controlplanelog.Info("default", "name", r.Name)
}

//+kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1alpha1-rke2controlplane,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=create;update,versions=v1alpha1,name=vrke2controlplane.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &RKE2ControlPlane{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlane) ValidateCreate() error {
	rke2controlplanelog.Info("validate create", "name", r.Name)

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlane) ValidateUpdate(old runtime.Object) error {
	rke2controlplanelog.Info("validate update", "name", r.Name)

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlane) ValidateDelete() error {
	rke2controlplanelog.Info("validate delete", "name", r.Name)

	return nil
}
