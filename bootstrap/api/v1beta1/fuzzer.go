/*
Copyright 2025 SUSE LLC.

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
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/randfill"
)

// FuzzFuncsv1beta1 exposes fuzzers for testing conversion logic.
func FuzzFuncsv1beta1(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		RKE2ConfigFuzzer,
		RKE2ConfigTemplateFuzzer,
	}
}

// RKE2ConfigFuzzer is fuzzer for v1beta1 RKE2Config (hub).
func RKE2ConfigFuzzer(obj *RKE2Config, c randfill.Continue) {
	c.FillNoCustom(obj)
	val := c.Bool()
	obj.Spec.GzipUserData = &val
}

// RKE2ConfigTemplateFuzzer is fuzzer for v1beta1 RKE2ConfigTemplate (hub).
func RKE2ConfigTemplateFuzzer(obj *RKE2ConfigTemplate, c randfill.Continue) {
	c.FillNoCustom(obj)
	val := c.Bool()
	obj.Spec.Template.Spec.GzipUserData = &val
}
