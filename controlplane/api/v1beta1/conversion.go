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

package v1beta1

// Hub marks RKE2ControlPlane as a conversion hub.
func (*RKE2ControlPlane) Hub() {}

// Hub marks RKE2ControlPlaneList as a conversion hub.
func (*RKE2ControlPlaneList) Hub() {}

// Hub marks RKE2ControlPlaneSpec as a conversion hub.
func (*RKE2ControlPlaneSpec) Hub() {}

// Hub marks RKE2ControlPlaneTemplate as a conversion hub.
func (*RKE2ControlPlaneTemplate) Hub() {}

// Hub marks RKE2ControlPlaneTemplateList as a conversion hub.
func (*RKE2ControlPlaneTemplateList) Hub() {}
