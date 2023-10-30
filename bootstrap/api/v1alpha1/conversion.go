/*
Copyright 2023 SUSE.

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

	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha2"
)

func (src *RKE2Config) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.RKE2Config)
	if !ok {
		return fmt.Errorf("not a RKE2Config: %v", dst)
	}

	if err := Convert_v1alpha1_RKE2Config_To_v1alpha2_RKE2Config(src, dst, nil); err != nil {
		return err
	}

	return nil
}

func (dst *RKE2Config) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.RKE2Config)
	if !ok {
		return fmt.Errorf("not a RKE2Config: %v", src)
	}

	if err := Convert_v1alpha2_RKE2Config_To_v1alpha1_RKE2Config(src, dst, nil); err != nil {
		return err
	}

	return nil
}

func (src *RKE2ConfigTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.RKE2ConfigTemplate)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigTemplate: %v", dst)
	}

	if err := Convert_v1alpha1_RKE2ConfigTemplate_To_v1alpha2_RKE2ConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	return nil
}

func (dst *RKE2ConfigTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.RKE2ConfigTemplate)
	if !ok {
		return fmt.Errorf("not a RKE2ConfigTemplate: %v", dst)
	}

	if err := Convert_v1alpha2_RKE2ConfigTemplate_To_v1alpha1_RKE2ConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	return nil
}
