/*
Copyright 2026 SUSE LLC.

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

package v1beta2

import "k8s.io/apimachinery/pkg/runtime"

// NOTE: delete this file when v1beta2 is promoted and automatic generation
// is enabled.

// DeepCopyObject implements runtime.Object.
func (in *RKE2Config) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopyObject implements runtime.Object.
func (in *RKE2ConfigList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopyObject implements runtime.Object.
func (in *RKE2ConfigTemplate) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopyObject implements runtime. Object.
func (in *RKE2ConfigTemplateList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}
