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

package v1alpha1

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	apiconversion "k8s.io/apimachinery/pkg/conversion"

	utilconversion "sigs.k8s.io/cluster-api/util/conversion"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
)

func (src *RKE2Config) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.RKE2Config)
	if !ok {
		return fmt.Errorf("not a RKE2Config: %v", dst)
	}

	if err := Convert_v1alpha1_RKE2Config_To_v1beta1_RKE2Config(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &bootstrapv1.RKE2Config{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.AgentConfig.AirGappedChecksum != "" {
		dst.Spec.AgentConfig.AirGappedChecksum = restored.Spec.AgentConfig.AirGappedChecksum
	}

	if restored.Spec.AgentConfig.PodSecurityAdmissionConfigFile != "" {
		dst.Spec.AgentConfig.PodSecurityAdmissionConfigFile = restored.Spec.AgentConfig.PodSecurityAdmissionConfigFile
	}

	if restored.Spec.GzipUserData != nil {
		dst.Spec.GzipUserData = restored.Spec.GzipUserData
	}

	return nil
}

func (dst *RKE2Config) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.RKE2Config)
	if !ok {
		return fmt.Errorf("not a RKE2Config: %v", src)
	}

	if err := Convert_v1beta1_RKE2Config_To_v1alpha1_RKE2Config(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

func (src *RKE2ConfigList) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.RKE2ConfigList)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigList: %v", src)
	}

	return Convert_v1alpha1_RKE2ConfigList_To_v1beta1_RKE2ConfigList(src, dst, nil)
}

func (dst *RKE2ConfigList) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.RKE2ConfigList)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigList: %v", src)
	}

	return Convert_v1beta1_RKE2ConfigList_To_v1alpha1_RKE2ConfigList(src, dst, nil)
}

func (src *RKE2ConfigTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.RKE2ConfigTemplate)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigTemplate: %v", dst)
	}

	if err := Convert_v1alpha1_RKE2ConfigTemplate_To_v1beta1_RKE2ConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &bootstrapv1.RKE2ConfigTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.Template.Spec.AgentConfig.AirGappedChecksum != "" {
		dst.Spec.Template.Spec.AgentConfig.AirGappedChecksum = restored.Spec.Template.Spec.AgentConfig.AirGappedChecksum
	}

	if restored.Spec.Template.Spec.AgentConfig.PodSecurityAdmissionConfigFile != "" {
		dst.Spec.Template.Spec.AgentConfig.PodSecurityAdmissionConfigFile = restored.Spec.Template.Spec.AgentConfig.PodSecurityAdmissionConfigFile
	}

	if restored.Spec.Template.Spec.GzipUserData != nil {
		dst.Spec.Template.Spec.GzipUserData = restored.Spec.Template.Spec.GzipUserData
	}

	return nil
}

func (dst *RKE2ConfigTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.RKE2ConfigTemplate)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigTemplate: %v", dst)
	}

	if err := Convert_v1beta1_RKE2ConfigTemplate_To_v1alpha1_RKE2ConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

func (src *RKE2ConfigTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.RKE2ConfigTemplateList)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigTemplateList: %v", dst)
	}

	return Convert_v1alpha1_RKE2ConfigTemplateList_To_v1beta1_RKE2ConfigTemplateList(src, dst, nil)
}

func (dst *RKE2ConfigTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.RKE2ConfigTemplateList)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigTemplateList: %v", dst)
	}

	return Convert_v1beta1_RKE2ConfigTemplateList_To_v1alpha1_RKE2ConfigTemplateList(src, dst, nil)
}

func Convert_v1alpha1_RKE2AgentConfig_To_v1beta1_RKE2AgentConfig(in *RKE2AgentConfig, out *bootstrapv1.RKE2AgentConfig, s apiconversion.Scope) error {
	// We have to invoke conversion manually because of the removed RKE2Config.Spec.Version field.
	return autoConvert_v1alpha1_RKE2AgentConfig_To_v1beta1_RKE2AgentConfig(in, out, s)
}

func Convert_v1beta1_RKE2AgentConfig_To_v1alpha1_RKE2AgentConfig(in *bootstrapv1.RKE2AgentConfig, out *RKE2AgentConfig, s apiconversion.Scope) error {
	// We have to invoke conversion manually because of the added AirGappedChecksum field.
	return autoConvert_v1beta1_RKE2AgentConfig_To_v1alpha1_RKE2AgentConfig(in, out, s)
}

func Convert_v1alpha1_RKE2ConfigSpec_To_v1beta1_RKE2ConfigSpec(in *RKE2ConfigSpec, out *bootstrapv1.RKE2ConfigSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_RKE2ConfigSpec_To_v1beta1_RKE2ConfigSpec(in, out, s); err != nil {
		return err
	}

	// Default to false during up-conversion since field doesn't exist in v1alpha1
	out.GzipUserData = nil
	return nil
}

func Convert_v1beta1_RKE2ConfigSpec_To_v1alpha1_RKE2ConfigSpec(in *bootstrapv1.RKE2ConfigSpec, out *RKE2ConfigSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1beta1_RKE2ConfigSpec_To_v1alpha1_RKE2ConfigSpec(in, out, s); err != nil {
		return err
	}

	// GzipUserData does not exist in v1alpha1, so it's intentionally ignored
	return nil
}

func Convert_v1beta1_FileSource_To_v1alpha1_FileSource(in *bootstrapv1.FileSource, out *FileSource, s apiconversion.Scope) error {
	// Only Secret is supported in v1alpha1.
	// If ConfigMap is set in v1beta1, it will be ignored during this conversion since it does not exist in v1alpha1.
	if in.ConfigMap.Name != "" || in.ConfigMap.Key != "" {
		in.ConfigMap = bootstrapv1.FileSourceRef{}
	}

	if err := autoConvert_v1beta1_FileSource_To_v1alpha1_FileSource(in, out, s); err != nil {
		return err
	}
	return nil
}

func Convert_v1alpha1_SecretFileSource_To_v1beta1_FileSourceRef(in *SecretFileSource, out *bootstrapv1.FileSourceRef, s apiconversion.Scope) error {
	out.Name = in.Name
	out.Key = in.Key
	return nil
}

func Convert_v1beta1_FileSourceRef_To_v1alpha1_SecretFileSource(in *bootstrapv1.FileSourceRef, out *SecretFileSource, s apiconversion.Scope) error {
	out.Name = in.Name
	out.Key = in.Key
	return nil
}
