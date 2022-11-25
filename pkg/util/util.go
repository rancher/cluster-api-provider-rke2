/*
Copyright 2022 SUSE.

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

package util

import (
	"context"

	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetOwnerControlPlane(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*controlplanev1.RKE2ControlPlane, error) {

	ownerRefs := obj.OwnerReferences
	var cpOwnerRef metav1.OwnerReference
	for _, ownerRef := range ownerRefs {
		if ownerRef.APIVersion == controlplanev1.GroupVersion.Group && ownerRef.Kind == "RKE2ControlPlane" {
			cpOwnerRef = ownerRef
		}
	}

	if (cpOwnerRef != metav1.OwnerReference{}) {
		return GetControlPlaneByName(ctx, c, obj.Namespace, cpOwnerRef.Name)
	}

	return nil, nil
}

// GetControlPlaneByName finds and return a ControlPlane object using the specified params.
func GetControlPlaneByName(ctx context.Context, c client.Client, namespace, name string) (*controlplanev1.RKE2ControlPlane, error) {
	m := &controlplanev1.RKE2ControlPlane{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, m); err != nil {
		return nil, err
	}
	return m, nil
}

// GetClusterByName finds and return a Cluster object using the specified params.
func GetClusterByName(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.Cluster, error) {
	m := &clusterv1.Cluster{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, m); err != nil {
		return nil, err
	}
	return m, nil
}
