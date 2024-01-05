/*
Copyright 2024 SUSE.

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
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1beta1"
	//utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts the v1alpha1 RKE2ControlPlane receiver to a v1beta1 RKEControlPlane.
func (r *RKE2ControlPlane) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.RKE2ControlPlane)

	if err := autoConvert_v1alpha1_RKE2ControlPlane_To_v1beta1_RKE2ControlPlane(r, dst, nil); err != nil {
		return err
	}

	// restored := &controlplanev1.RKE2ControlPlane{}
	// if ok, err := utilconversion.UnmarshalData(r, restored); err != nil || !ok {
	// 	return err
	// }
	//dst.Spec.XXX = restored.Spec.XXXX

	return nil
}

// ConvertFrom converts the v1beta1 RKE2ControlPlane receiver to a v1alpha1 RKE2ControlPlane.
func (r *RKE2ControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.RKE2ControlPlane)

	if err := autoConvert_v1beta1_RKE2ControlPlane_To_v1alpha1_RKE2ControlPlane(src, r, nil); err != nil {
		return err
	}

	// if err := utilconversion.MarshalData(src, r); err != nil {
	// 	return nil
	// }

	return nil
}

// ConvertTo converts the v1alpha1 RKE2ControlPlaneList receiver to a v1beta1 RKEControlPlaneList.
func (r *RKE2ControlPlaneList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.RKE2ControlPlaneList)

	return autoConvert_v1alpha1_RKE2ControlPlaneList_To_v1beta1_RKE2ControlPlaneList(r, dst, nil)
}

// ConvertFrom converts the v1beta1 RKE2ControlPlaneList receiver to a v1alpha1 RKE2ControlPlaneList.
func (r *RKE2ControlPlaneList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.RKE2ControlPlaneList)

	return autoConvert_v1beta1_RKE2ControlPlaneList_To_v1alpha1_RKE2ControlPlaneList(src, r, nil)
}
