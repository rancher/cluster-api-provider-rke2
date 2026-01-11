package v1beta2

import "k8s.io/apimachinery/pkg/runtime"

// NOTE: delete this file when v1beta2 is promoted and automatic generation
// is enabled.

// DeepCopyObject implements runtime.Object.
func (in *RKE2ControlPlane) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopyObject implements runtime.Object.
func (in *RKE2ControlPlaneList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopyObject implements runtime.Object.
func (in *RKE2ControlPlaneTemplate) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopyObject implements runtime. Object.
func (in *RKE2ControlPlaneTemplateList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}
