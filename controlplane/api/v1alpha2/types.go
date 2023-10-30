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

package v1alpha2

// RegistrationMethod defines the methods to use for registering a new node in a cluster.
type RegistrationMethod string

var (
	// RegistrationMethodFavourInternalIPs is a registration method where the IP address of the control plane
	// machines are used for registration. For each machine it will check if there is an internal IP address
	// and will use that. If there is no internal IP address it will use the external IP address if there is one.
	RegistrationMethodFavourInternalIPs = RegistrationMethod("internal-first")
	// RegistrationMethodInternalIPs is a registration method where the internal IP address of the control plane
	// machines are used for registration.
	RegistrationMethodInternalIPs = RegistrationMethod("internal-only-ips")
	// RegistrationMethodExternalIPs is a registration method where the external IP address of the control plane
	// machines are used for registration.
	RegistrationMethodExternalIPs = RegistrationMethod("external-only-ips")
	// RegistrationMethodAddress is a registration method where an explicit address supplied at cluster creation
	// time is used for registration. This is for use in LB or VIP scenarios.
	RegistrationMethodAddress = RegistrationMethod("address")
)
