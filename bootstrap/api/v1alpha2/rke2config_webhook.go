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
	"fmt"

	"github.com/coreos/butane/config/common"
	fcos "github.com/coreos/butane/config/fcos/v1_4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	cannotUseWithIgnition = fmt.Sprintf("not supported when spec.format is set to %q", Ignition)
	rke2configlog         = logf.Log.WithName("rke2config-resource")
)

// SetupWebhookWithManager sets up and registers the webhook with the manager.
func (r *RKE2Config) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-bootstrap-cluster-x-k8s-io-v1alpha2-rke2config,mutating=true,failurePolicy=fail,sideEffects=None,groups=bootstrap.cluster.x-k8s.io,resources=rke2configs,verbs=create;update,versions=v1alpha2,name=mrke2config.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &RKE2Config{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *RKE2Config) Default() {
	DefaultRKE2ConfigSpec(&r.Spec)
}

// DefaultRKE2ConfigSpec defaults the RKE2ConfigSpec.
func DefaultRKE2ConfigSpec(spec *RKE2ConfigSpec) {
	if spec.AgentConfig.Format == "" {
		spec.AgentConfig.Format = CloudConfig
	}
}

//+kubebuilder:webhook:path=/validate-bootstrap-cluster-x-k8s-io-v1alpha2-rke2config,mutating=false,failurePolicy=fail,sideEffects=None,groups=bootstrap.cluster.x-k8s.io,resources=rke2configs,verbs=create;update,versions=v1alpha2,name=vrke2config.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &RKE2Config{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2Config) ValidateCreate() (admission.Warnings, error) {
	rke2configlog.Info("RKE2Config validate create", "rke2config", klog.KObj(r))

	var allErrs field.ErrorList

	allErrs = append(allErrs, ValidateRKE2ConfigSpec(r.Name, &r.Spec)...)

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2Config").GroupKind(), r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2Config) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	rke2configlog.Info("RKE2Config validate update", "rke2config", klog.KObj(r))

	var allErrs field.ErrorList

	allErrs = append(allErrs, ValidateRKE2ConfigSpec(r.Name, &r.Spec)...)

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2Config").GroupKind(), r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2Config) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

// ValidateRKE2ConfigSpec validates the RKE2ConfigSpec.
func ValidateRKE2ConfigSpec(_ string, spec *RKE2ConfigSpec) field.ErrorList {
	allErrs := spec.validate(field.NewPath("spec"))

	if len(allErrs) == 0 {
		return nil
	}

	return allErrs
}

func (s *RKE2ConfigSpec) validate(pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, s.validateIgnition(pathPrefix)...)

	return allErrs
}

func (s *RKE2ConfigSpec) validateIgnition(pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if s.AgentConfig.Format == Ignition {
		_, reports, _ := fcos.ToIgn3_3Bytes([]byte(s.AgentConfig.AdditionalUserData.Config), common.TranslateBytesOptions{})
		if (len(reports.Entries) > 0 && s.AgentConfig.AdditionalUserData.Strict) || reports.IsFatal() {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("agentConfig.AdditionalUserData.Config"),
					s.AgentConfig.AdditionalUserData.Config,
					fmt.Sprintf("error parsing Butane config: %v", reports.String()),
				),
			)
		}
	}

	for i, file := range s.Files {
		if file.Encoding == Gzip || file.Encoding == GzipBase64 {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("files").Index(i).Child("encoding"),
					cannotUseWithIgnition,
				),
			)
		}
	}

	return allErrs
}
