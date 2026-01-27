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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1" // nolint:staticcheck
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"

	"github.com/pkg/errors"
	bootstrapv1beta1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
)

// apiVersionGetter is specifically used for conversion testing.
var apiVersionGetter = func(_ schema.GroupKind) (string, error) {
	return "", errors.New("apiVersionGetter not set")
}

// SetAPIVersionGetter sets a valid api version for conversion testing.
func SetAPIVersionGetter(f func(gk schema.GroupKind) (string, error)) {
	apiVersionGetter = f
}

// ConvertTo converts this RKE2ControlPlane to the Hub version (v1beta2).
func (src *RKE2ControlPlane) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*controlplanev1.RKE2ControlPlane)
	if !ok {
		return fmt.Errorf("not a RKE2ControlPlane: %v", dst)
	}

	if err := Convert_v1beta1_RKE2ControlPlane_To_v1beta2_RKE2ControlPlane(src, dst, nil); err != nil {
		return err
	}

	infraRef, err := convertToContractVersionedObjectReference(&src.Spec.MachineTemplate.InfrastructureRef)
	if err != nil {
		return err
	}
	dst.Spec.MachineTemplate.Spec.InfrastructureRef = *infraRef

	// Preserve Hub data on down-conversion.
	restored := &controlplanev1.RKE2ControlPlane{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := controlplanev1.RKE2ControlPlaneInitializationStatus{}
	restoredControlPlaneInitialized := restored.Status.Initialization.ControlPlaneInitialized
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Initialized, ok, restoredControlPlaneInitialized, &initialization.ControlPlaneInitialized)
	if !reflect.DeepEqual(initialization, controlplanev1.RKE2ControlPlaneInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1beta1).
func (dst *RKE2ControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*controlplanev1.RKE2ControlPlane)
	if !ok {
		return fmt.Errorf("not a RKE2ControlPlane: %v", src)
	}

	if err := Convert_v1beta2_RKE2ControlPlane_To_v1beta1_RKE2ControlPlane(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.MachineTemplate.Spec.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(&src.Spec.MachineTemplate.Spec.InfrastructureRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.MachineTemplate.InfrastructureRef = *infraRef
	}

	// Preserve Hub data on down-conversion.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this RKE2ControlPlaneList to the Hub version (v1beta2).
func (src *RKE2ControlPlaneList) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*controlplanev1.RKE2ControlPlaneList)
	if !ok {
		return fmt.Errorf("not a RKE2ControlPlaneList: %v", dst)
	}

	return Convert_v1beta1_RKE2ControlPlaneList_To_v1beta2_RKE2ControlPlaneList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1beta1).
func (dst *RKE2ControlPlaneList) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*controlplanev1.RKE2ControlPlaneList)
	if !ok {
		return fmt.Errorf("not a RKE2ControlPlaneList: %v", src)
	}

	return Convert_v1beta2_RKE2ControlPlaneList_To_v1beta1_RKE2ControlPlaneList(src, dst, nil)
}

// ConvertTo converts this RKE2ControlPlaneTemplate to the Hub version (v1beta2).
func (src *RKE2ControlPlaneTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*controlplanev1.RKE2ControlPlaneTemplate)
	if !ok {
		return fmt.Errorf("not a RKE2ControlPlaneTemplate:  %v", dst)
	}

	if err := Convert_v1beta1_RKE2ControlPlaneTemplate_To_v1beta2_RKE2ControlPlaneTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	restored := &controlplanev1.RKE2ControlPlaneTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1beta1).
func (dst *RKE2ControlPlaneTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*controlplanev1.RKE2ControlPlaneTemplate)
	if !ok {
		return fmt.Errorf("not a RKE2ControlPlaneTemplate: %v", src)
	}

	if err := Convert_v1beta2_RKE2ControlPlaneTemplate_To_v1beta1_RKE2ControlPlaneTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this RKE2ControlPlaneTemplateList to the Hub version (v1beta2).
func (src *RKE2ControlPlaneTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*controlplanev1.RKE2ControlPlaneTemplateList)
	if !ok {
		return fmt.Errorf("not a RKE2ControlPlaneTemplateList: %v", dst)
	}

	return Convert_v1beta1_RKE2ControlPlaneTemplateList_To_v1beta2_RKE2ControlPlaneTemplateList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1beta1).
func (dst *RKE2ControlPlaneTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*controlplanev1.RKE2ControlPlaneTemplateList)
	if !ok {
		return fmt.Errorf("not a RKE2ControlPlaneTemplateList:  %v", src)
	}

	return Convert_v1beta2_RKE2ControlPlaneTemplateList_To_v1beta1_RKE2ControlPlaneTemplateList(src, dst, nil)
}

// Convert_v1beta1_RKE2ControlPlaneStatus_To_v1beta2_RKE2ControlPlaneStatus handles manual conversion of status fields.
func Convert_v1beta1_RKE2ControlPlaneStatus_To_v1beta2_RKE2ControlPlaneStatus(in *RKE2ControlPlaneStatus, out *controlplanev1.RKE2ControlPlaneStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_RKE2ControlPlaneStatus_To_v1beta2_RKE2ControlPlaneStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: old replica counters should not be automatically be converted into replica counters with a new semantic.
	out.ReadyReplicas = nil

	// Retrieve new conditions (v1beta2) and replica counter from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		out.ReadyReplicas = in.V1Beta2.ReadyReplicas
		out.AvailableReplicas = in.V1Beta2.AvailableReplicas
		out.UpToDateReplicas = in.V1Beta2.UpToDateReplicas
	}

	// Move legacy conditions (v1beta1), failureReason, failureMessage and replica counters to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &controlplanev1.RKE2ControlPlaneDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &controlplanev1.RKE2ControlPlaneV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		clusterv1beta1.Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	out.Deprecated.V1Beta1.UpdatedReplicas = in.UpdatedReplicas
	out.Deprecated.V1Beta1.ReadyReplicas = in.ReadyReplicas
	out.Deprecated.V1Beta1.UnavailableReplicas = in.UnavailableReplicas

	// Move `initialized` to `ControlPlaneInitialized` is implemented in `ConvertTo`.

	return nil
}

// Convert_v1beta2_RKE2ControlPlaneStatus_To_v1beta1_RKE2ControlPlaneStatus handles manual conversion of status fields.
func Convert_v1beta2_RKE2ControlPlaneStatus_To_v1beta1_RKE2ControlPlaneStatus(in *controlplanev1.RKE2ControlPlaneStatus, out *RKE2ControlPlaneStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_RKE2ControlPlaneStatus_To_v1beta1_RKE2ControlPlaneStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	out.ReadyReplicas = 0

	// Retrieve legacy conditions (v1beta1), failureReason, failureMessage and replica counters from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			clusterv1beta1.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
		out.UpdatedReplicas = in.Deprecated.V1Beta1.UpdatedReplicas
		out.ReadyReplicas = in.Deprecated.V1Beta1.ReadyReplicas
		out.UnavailableReplicas = in.Deprecated.V1Beta1.UnavailableReplicas
	}

	out.Initialized = ptr.Deref(in.Initialization.ControlPlaneInitialized, false)
	out.Ready = out.ReadyReplicas > 0

	// Move new conditions (v1beta2) and replica counter to the v1beta2 field.
	if in.Conditions == nil && in.ReadyReplicas == nil && in.AvailableReplicas == nil && in.UpToDateReplicas == nil {
		return nil
	}
	out.V1Beta2 = &RKE2ControlPlaneV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	out.V1Beta2.ReadyReplicas = in.ReadyReplicas
	out.V1Beta2.AvailableReplicas = in.AvailableReplicas
	out.V1Beta2.UpToDateReplicas = in.UpToDateReplicas

	return nil
}

// Convert_v1beta1_RKE2ControlPlaneSpec_To_v1beta2_RKE2ControlPlaneSpec handles spec conversion.
func Convert_v1beta1_RKE2ControlPlaneSpec_To_v1beta2_RKE2ControlPlaneSpec(in *RKE2ControlPlaneSpec, out *controlplanev1.RKE2ControlPlaneSpec, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta1_RKE2ControlPlaneSpec_To_v1beta2_RKE2ControlPlaneSpec(in, out, s)
}

// Convert_v1beta2_RKE2ControlPlaneSpec_To_v1beta1_RKE2ControlPlaneSpec handles spec conversion.
func Convert_v1beta2_RKE2ControlPlaneSpec_To_v1beta1_RKE2ControlPlaneSpec(in *controlplanev1.RKE2ControlPlaneSpec, out *RKE2ControlPlaneSpec, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta2_RKE2ControlPlaneSpec_To_v1beta1_RKE2ControlPlaneSpec(in, out, s)
}

func Convert_v1beta1_RKE2ControlPlaneTemplate_To_v1beta2_RKE2ControlPlaneTemplate(in *RKE2ControlPlaneTemplate, out *controlplanev1.RKE2ControlPlaneTemplate, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta1_RKE2ControlPlaneTemplate_To_v1beta2_RKE2ControlPlaneTemplate(in, out, s)
}

func Convert_v1beta1_RKE2ControlPlaneMachineTemplate_To_v1beta2_RKE2ControlPlaneMachineTemplate(in *RKE2ControlPlaneMachineTemplate, out *controlplanev1.RKE2ControlPlaneMachineTemplate, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_RKE2ControlPlaneMachineTemplate_To_v1beta2_RKE2ControlPlaneMachineTemplate(in, out, s); err != nil {
		return err
	}

	// convert `infrastructureRef` of type `ObjectReference` to `machineTemplate.spec.infrastructureRef` of type `ContractVersionedObjectReference`.
	infraRef, err := convertToContractVersionedObjectReference(&in.InfrastructureRef)
	if err != nil {
		return err
	}
	out.Spec.InfrastructureRef = *infraRef

	// convert timeouts of type `*metav1.Duration` to seconds of type `*int32`.
	out.Spec.Deletion.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.Spec.Deletion.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)

	return nil
}

func Convert_v1beta2_RKE2ControlPlaneMachineTemplate_To_v1beta1_RKE2ControlPlaneMachineTemplate(in *controlplanev1.RKE2ControlPlaneMachineTemplate, out *RKE2ControlPlaneMachineTemplate, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_RKE2ControlPlaneMachineTemplate_To_v1beta1_RKE2ControlPlaneMachineTemplate(in, out, s); err != nil {
		return err
	}

	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.Spec.Deletion.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.Spec.Deletion.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.Spec.Deletion.NodeDeletionTimeoutSeconds)

	return nil
}

func Convert_v1beta1_RKE2ConfigSpec_To_v1beta2_RKE2ConfigSpec(in *bootstrapv1beta1.RKE2ConfigSpec, out *bootstrapv1.RKE2ConfigSpec, s apimachineryconversion.Scope) error {
	return bootstrapv1beta1.Convert_v1beta1_RKE2ConfigSpec_To_v1beta2_RKE2ConfigSpec(in, out, s)
}

func Convert_v1beta2_RKE2ConfigSpec_To_v1beta1_RKE2ConfigSpec(in *bootstrapv1.RKE2ConfigSpec, out *bootstrapv1beta1.RKE2ConfigSpec, s apimachineryconversion.Scope) error {
	return bootstrapv1beta1.Convert_v1beta2_RKE2ConfigSpec_To_v1beta1_RKE2ConfigSpec(in, out, s)
}

// After switching to `ContractVersionedObjectReference`, there needs to be a conversion to and from `ObjectReference`.
func convertToContractVersionedObjectReference(ref *corev1.ObjectReference) (*clusterv1.ContractVersionedObjectReference, error) {
	var apiGroup string
	if ref.APIVersion != "" {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to convert object: failed to parse apiVersion: %v", err)
		}
		apiGroup = gv.Group
	}
	return &clusterv1.ContractVersionedObjectReference{
		APIGroup: apiGroup,
		Kind:     ref.Kind,
		Name:     ref.Name,
	}, nil
}

func convertToObjectReference(ref *clusterv1.ContractVersionedObjectReference, namespace string) (*corev1.ObjectReference, error) {
	apiVersion, err := apiVersionGetter(schema.GroupKind{
		Group: ref.APIGroup,
		Kind:  ref.Kind,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to convert object: %v", err)
	}
	return &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       ref.Kind,
		Namespace:  namespace,
		Name:       ref.Name,
	}, nil
}

// Need to implement conversion between v1beta1 and v1beta2 ObjectMeta
// The logic is built in upstream CAPI but we need to have a convert function here so that controller-gen is aware of it.
func Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in *clusterv1beta1.ObjectMeta, out *clusterv1.ObjectMeta, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in, out, s)
}

func Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in *clusterv1.ObjectMeta, out *clusterv1beta1.ObjectMeta, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in, out, s)
}

// Need to implement condition conversion between old CAPI-specific condition and the generic from apimachinery.
// The logic is built in upstream CAPI but we need to have a convert function here so that controller-gen is aware of it.
func Convert_v1beta1_Condition_To_v1_Condition(in *clusterv1beta1.Condition, out *metav1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_Condition_To_v1_Condition(in, out, s)
}

func Convert_v1_Condition_To_v1beta1_Condition(in *metav1.Condition, out *clusterv1beta1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1_Condition_To_v1beta1_Condition(in, out, s)
}
