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

	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *RKE2ControlPlane) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*controlplanev1.RKE2ControlPlane)
	if !ok {
		return fmt.Errorf("not a RKE2ControlPlane: %v", dst)
	}

	if err := Convert_v1alpha1_RKE2ControlPlane_To_v1alpha2_RKE2ControlPlane(src, dst, nil); err != nil {
		return err
	}

	return nil
}

func (dst *RKE2ControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*controlplanev1.RKE2ControlPlane)
	if !ok {
		return fmt.Errorf("not a RKE2ControlPlane: %v", src)
	}

	if err := Convert_v1alpha2_RKE2ControlPlane_To_v1alpha1_RKE2ControlPlane(src, dst, nil); err != nil {
		return err
	}

	return nil
}
