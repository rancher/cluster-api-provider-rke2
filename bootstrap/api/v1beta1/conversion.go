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
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/ptr"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1" // nolint:staticcheck
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
)

// ConvertTo converts this RKE2Config to the Hub version (v1beta2).
func (src *RKE2Config) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.RKE2Config)
	if !ok {
		return fmt.Errorf("not a RKE2Config:  %v", dst)
	}

	if err := Convert_v1beta1_RKE2Config_To_v1beta2_RKE2Config(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	restored := &bootstrapv1.RKE2Config{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := bootstrapv1.RKE2ConfigInitializationStatus{}
	restoredBootstrapDataSecretCreated := restored.Status.Initialization.DataSecretCreated
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok, restoredBootstrapDataSecretCreated, &initialization.DataSecretCreated)
	if !reflect.DeepEqual(initialization, bootstrapv1.RKE2ConfigInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1beta1).
func (dst *RKE2Config) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.RKE2Config)
	if !ok {
		return fmt.Errorf("not a RKE2Config: %v", src)
	}

	if err := Convert_v1beta2_RKE2Config_To_v1beta1_RKE2Config(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this RKE2ConfigList to the Hub version (v1beta2).
func (src *RKE2ConfigList) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.RKE2ConfigList)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigList: %v", dst)
	}

	return Convert_v1beta1_RKE2ConfigList_To_v1beta2_RKE2ConfigList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1beta1).
func (dst *RKE2ConfigList) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.RKE2ConfigList)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigList: %v", src)
	}

	return Convert_v1beta2_RKE2ConfigList_To_v1beta1_RKE2ConfigList(src, dst, nil)
}

// ConvertTo converts this RKE2ConfigTemplate to the Hub version (v1beta2).
func (src *RKE2ConfigTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.RKE2ConfigTemplate)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigTemplate: %v", dst)
	}

	if err := Convert_v1beta1_RKE2ConfigTemplate_To_v1beta2_RKE2ConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	restored := &bootstrapv1.RKE2ConfigTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1beta1).
func (dst *RKE2ConfigTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.RKE2ConfigTemplate)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigTemplate:  %v", src)
	}

	if err := Convert_v1beta2_RKE2ConfigTemplate_To_v1beta1_RKE2ConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this RKE2ConfigTemplateList to the Hub version (v1beta2).
func (src *RKE2ConfigTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.RKE2ConfigTemplateList)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigTemplateList: %v", dst)
	}

	return Convert_v1beta1_RKE2ConfigTemplateList_To_v1beta2_RKE2ConfigTemplateList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1beta1).
func (dst *RKE2ConfigTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.RKE2ConfigTemplateList)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigTemplateList: %v", src)
	}

	return Convert_v1beta2_RKE2ConfigTemplateList_To_v1beta1_RKE2ConfigTemplateList(src, dst, nil)
}

// Convert_v1beta1_RKE2ConfigStatus_To_v1beta2_RKE2ConfigStatus converts RKE2ConfigStatus from v1beta1 to v1beta2.
func Convert_v1beta1_RKE2ConfigStatus_To_v1beta2_RKE2ConfigStatus(in *RKE2ConfigStatus, out *bootstrapv1.RKE2ConfigStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_RKE2ConfigStatus_To_v1beta2_RKE2ConfigStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
	}

	// Move legacy conditions (v1beta1), failureReason and failureMessage to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &bootstrapv1.RKE2ConfigDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &bootstrapv1.RKE2ConfigV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		clusterv1beta1.Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage

	return nil
}

// Convert_v1beta2_RKE2ConfigStatus_To_v1beta1_RKE2ConfigStatus converts RKE2ConfigStatus from v1beta2 to v1beta1.
func Convert_v1beta2_RKE2ConfigStatus_To_v1beta1_RKE2ConfigStatus(in *bootstrapv1.RKE2ConfigStatus, out *RKE2ConfigStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_RKE2ConfigStatus_To_v1beta1_RKE2ConfigStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1), failureReason and failureMessage from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			clusterv1beta1.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
	}

	// Move initialization to old fields
	out.Ready = ptr.Deref(in.Initialization.DataSecretCreated, false)

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}
	out.V1Beta2 = &RKE2ConfigV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions

	return nil
}

// Convert_v1beta1_RKE2AgentConfig_To_v1beta2_RKE2AgentConfig converts RKE2AgentConfig from v1beta1 to v1beta2.
func Convert_v1beta1_RKE2AgentConfig_To_v1beta2_RKE2AgentConfig(in *RKE2AgentConfig, out *bootstrapv1.RKE2AgentConfig, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta1_RKE2AgentConfig_To_v1beta2_RKE2AgentConfig(in, out, s)
}

// Convert_v1beta2_RKE2AgentConfig_To_v1beta1_RKE2AgentConfig converts RKE2AgentConfig from v1beta2 to v1beta1.
func Convert_v1beta2_RKE2AgentConfig_To_v1beta1_RKE2AgentConfig(in *bootstrapv1.RKE2AgentConfig, out *RKE2AgentConfig, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta2_RKE2AgentConfig_To_v1beta1_RKE2AgentConfig(in, out, s)
}

// Convert_v1beta1_RKE2ConfigSpec_To_v1beta2_RKE2ConfigSpec converts RKE2ConfigSpec from v1beta1 to v1beta2.
func Convert_v1beta1_RKE2ConfigSpec_To_v1beta2_RKE2ConfigSpec(in *RKE2ConfigSpec, out *bootstrapv1.RKE2ConfigSpec, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta1_RKE2ConfigSpec_To_v1beta2_RKE2ConfigSpec(in, out, s)
}

// Convert_v1beta2_RKE2ConfigSpec_To_v1beta1_RKE2ConfigSpec converts RKE2ConfigSpec from v1beta2 to v1beta1.
func Convert_v1beta2_RKE2ConfigSpec_To_v1beta1_RKE2ConfigSpec(in *bootstrapv1.RKE2ConfigSpec, out *RKE2ConfigSpec, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta2_RKE2ConfigSpec_To_v1beta1_RKE2ConfigSpec(in, out, s)
}

// Need to implement condition conversion between old CAPI-specific condition and the generic from apimachinery.
// The logic is built in upstream CAPI but we need to have a convert function here so that controller-gen is aware of it.
func Convert_v1beta1_Condition_To_v1_Condition(in *clusterv1beta1.Condition, out *metav1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_Condition_To_v1_Condition(in, out, s)
}

func Convert_v1_Condition_To_v1beta1_Condition(in *metav1.Condition, out *clusterv1beta1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1_Condition_To_v1beta1_Condition(in, out, s)
}
