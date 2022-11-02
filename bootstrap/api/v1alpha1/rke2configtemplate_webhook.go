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
var rke2configtemplatelog = logf.Log.WithName("rke2configtemplate-resource")

func (r *Rke2ConfigTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-bootstrap-cluster-x-k8s-io-v1alpha1-rke2configtemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=bootstrap.cluster.x-k8s.io,resources=rke2configtemplates,verbs=create;update,versions=v1alpha1,name=mrke2configtemplate.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Rke2ConfigTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Rke2ConfigTemplate) Default() {
	rke2configtemplatelog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-bootstrap-cluster-x-k8s-io-v1alpha1-rke2configtemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=bootstrap.cluster.x-k8s.io,resources=rke2configtemplates,verbs=create;update,versions=v1alpha1,name=vrke2configtemplate.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Rke2ConfigTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Rke2ConfigTemplate) ValidateCreate() error {
	rke2configtemplatelog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Rke2ConfigTemplate) ValidateUpdate(old runtime.Object) error {
	rke2configtemplatelog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Rke2ConfigTemplate) ValidateDelete() error {
	rke2configtemplatelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
