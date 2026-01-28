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
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/coreos/butane/config/common"
	fcos "github.com/coreos/butane/config/fcos/v1_4"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
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
	rke2ConfigLogger      = logf.Log.WithName("RKE2Config")
)

// RKE2ConfigCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind RKE2Config when those are created or updated.
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type RKE2ConfigCustomDefaulter struct{}

// RKE2ConfigCustomValidator struct is responsible for validating the RKE2Config resource
// when it is created, updated, or deleted.
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type RKE2ConfigCustomValidator struct{}

// SetupRKE2ConfigWebhookWithManager sets up the Controller Manager for the Webhook for the RKE2ControlPlaneTemplate resource.
func SetupRKE2ConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&RKE2Config{}).
		WithValidator(&RKE2ConfigCustomValidator{}).
		WithDefaulter(&RKE2ConfigCustomDefaulter{}, admission.DefaulterRemoveUnknownOrOmitableFields).
		Complete()
}

var _ webhook.CustomDefaulter = &RKE2ConfigCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *RKE2ConfigCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	rc, ok := obj.(*RKE2Config)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a RKE2Config but got a %T", obj))
	}

	rke2ConfigLogger.Info("defaulting", "RKE2Config", klog.KObj(rc))

	DefaultRKE2ConfigSpec(&rc.Spec)

	return nil
}

// DefaultRKE2ConfigSpec defaults the RKE2ConfigSpec.
func DefaultRKE2ConfigSpec(spec *RKE2ConfigSpec) {
	if spec.AgentConfig.Format == "" {
		spec.AgentConfig.Format = CloudConfig
	}

	if spec.AgentConfig.AdditionalUserData.Data != nil {
		if err := CorrectArbitraryData(spec.AgentConfig.AdditionalUserData.Data); err != nil {
			rke2ConfigLogger.Error(err, "failed to correct the additional user data for RKE2ConfigSpec")
		}
	}
}

var ignoredCloudInitFields = []string{"write_files", "ntp", "runcmd"}

// CorrectArbitraryData makes individual corrections to data and makes it YAML compliant.
func CorrectArbitraryData(arbitraryData map[string]string) error {
	// Remove ignored fields from the map
	for _, key := range ignoredCloudInitFields {
		delete(arbitraryData, key)
	}

	// Make individual corrections to each value
	for k, v := range arbitraryData {
		b := bytes.Buffer{}
		en := yaml.NewEncoder(&b)
		en.SetIndent(2)

		mapping := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(v), &mapping); err == nil {
			if err := en.Encode(&mapping); err != nil {
				return fmt.Errorf("invalid map value provided: '%s', error: %w", v, err)
			}

			ident := "\n  "
			arbitraryData[k] = ident + strings.ReplaceAll(b.String(), "\n", ident)

			continue
		}

		list := []interface{}{}
		if err := yaml.Unmarshal([]byte(v), &list); err == nil {
			if err := en.Encode(&list); err != nil {
				return fmt.Errorf("invalid list value provided: '%s', error: %w", v, err)
			}

			arbitraryData[k] = "\n" + b.String()

			continue
		}
	}

	return nil
}

var _ webhook.CustomValidator = &RKE2ConfigCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ConfigCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rc, ok := obj.(*RKE2Config)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2Config object but got %T", obj)
	}

	rke2ConfigLogger.Info("validate create", "RKE2Config", klog.KObj(rc))

	var allErrs field.ErrorList

	allErrs = append(allErrs, ValidateRKE2ConfigSpec(rc.Name, &rc.Spec)...)

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2Config").GroupKind(), rc.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ConfigCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	newrc, ok := newObj.(*RKE2Config)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2Config object but got %T", newObj)
	}

	rke2ConfigLogger.Info("validate update", "RKE2Config", klog.KObj(newrc))

	var allErrs field.ErrorList

	allErrs = append(allErrs, ValidateRKE2ConfigSpec(newrc.Name, &newrc.Spec)...)

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2Config").GroupKind(), newrc.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ConfigCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rc, ok := obj.(*RKE2Config)
	if !ok {
		return nil, fmt.Errorf("expected a RKE2Config object but got %T", obj)
	}

	rke2ConfigLogger.Info("validate delete", "RKE2Config", klog.KObj(rc))

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
	allErrs = append(allErrs, s.validateRegistries(pathPrefix)...)

	return allErrs
}

func (s *RKE2ConfigSpec) validateRegistries(pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	for regName, regConfig := range s.PrivateRegistriesConfig.Configs {
		if regConfig.AuthSecret == (v1.ObjectReference{}) && regConfig.TLS == (TLSConfig{}) {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("privateRegistriesConfig.configs."+regName),
					regConfig,
					"need either credentials, tls settings or both for registry: "+regName),
			)
		}
	}

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
	}

	return allErrs
}
