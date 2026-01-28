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

package rke2

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/collections"

	"github.com/rancher/cluster-api-provider-rke2/pkg/secret"
)

const (
	// DefaultWorkloadTimeout is the default timeout for the management cluster.
	DefaultWorkloadTimeout = 30 * time.Second
)

// ManagementCluster defines all behaviors necessary for something to function as a management cluster.
type ManagementCluster interface {
	ctrlclient.Reader

	GetMachinesForCluster(ctx context.Context, cluster *clusterv1.Cluster, filters ...collections.Func) (collections.Machines, error)
	GetWorkloadCluster(ctx context.Context, clusterKey ctrlclient.ObjectKey) (WorkloadCluster, error)
}

// Management holds operations on the management cluster.
type Management struct {
	Client              ctrlclient.Client
	SecretCachingClient ctrlclient.Reader
	ClusterCache        clustercache.ClusterCache
	EtcdDialTimeout     time.Duration
	EtcdCallTimeout     time.Duration
	EtcdLogger          *zap.Logger
}

// RemoteClusterConnectionError represents a failure to connect to a remote cluster.
type RemoteClusterConnectionError struct {
	Name string
	Err  error
}

func (e *RemoteClusterConnectionError) Error() string { return e.Name + ": " + e.Err.Error() }
func (e *RemoteClusterConnectionError) Unwrap() error { return e.Err }

// Get implements ctrlclient.Reader.
func (m *Management) Get(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
	return m.Client.Get(ctx, key, obj, opts...)
}

// List implements ctrlclient.Reader.
func (m *Management) List(ctx context.Context, list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
	return m.Client.List(ctx, list, opts...)
}

// GetMachinesForCluster returns a list of machines that can be filtered or not.
// If no filter is supplied then all machines associated with the target cluster are returned.
func (m *Management) GetMachinesForCluster(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	filters ...collections.Func,
) (collections.Machines, error) {
	return collections.GetFilteredMachinesForCluster(ctx, m.Client, cluster, filters...)
}

const (
	// RKE2ControlPlaneControllerName defines the controller used when creating clients.
	RKE2ControlPlaneControllerName = "rke2-controlplane-controller"
)

// GetWorkloadCluster builds a cluster object.
// The cluster comes with an etcd client generator to connect to any etcd pod living on a managed machine.
func (m *Management) GetWorkloadCluster(ctx context.Context, clusterKey ctrlclient.ObjectKey) (WorkloadCluster, error) {
	restConfig, err := remote.RESTConfig(ctx, RKE2ControlPlaneControllerName, m.Client, clusterKey)
	if err != nil {
		return nil, &RemoteClusterConnectionError{Name: clusterKey.String(), Err: err}
	}

	restConfig = rest.CopyConfig(restConfig)
	restConfig.Timeout = DefaultWorkloadTimeout

	c, err := ctrlclient.New(restConfig, ctrlclient.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, &RemoteClusterConnectionError{Name: clusterKey.String(), Err: err}
	}

	return m.NewWorkload(ctx, c, restConfig, clusterKey)
}

func (m *Management) getEtcdCAKeyPair(ctx context.Context, cl ctrlclient.Reader, clusterKey ctrlclient.ObjectKey) (*certs.KeyPair, error) {
	certificates := secret.Certificates{&secret.ManagedCertificate{
		Purpose:  secret.EtcdServerCA,
		External: true,
	}}
	secretName := secret.Name(clusterKey.Name, secret.EtcdServerCA)

	// Try to get the certificate via the cached ctrlclient.
	if err := certificates.Lookup(ctx, cl, clusterKey); err != nil {
		// Return error if we got an errors which is not a NotFound error.
		return nil, errors.Wrapf(err, "failed to get secret CA bundle; etcd CA bundle %s/%s", clusterKey.Namespace, secretName)
	}

	var keypair *certs.KeyPair

	if s, err := certificates[0].Lookup(ctx, cl, clusterKey); err != nil {
		return nil, errors.Wrapf(err, "failed to get secret; etcd CA bundle %s/%s", clusterKey.Namespace, secretName)
	} else if s == nil {
		log.FromContext(ctx).Info("Secret is empty, skipping etcd client creation")

		return keypair, nil
	}

	return certificates[0].GetKeyPair(), nil
}

func generateClientCert(caCertEncoded, caKeyEncoded []byte, clientKey *rsa.PrivateKey) (tls.Certificate, error) {
	caCert, err := certs.DecodeCertPEM(caCertEncoded)
	if err != nil {
		return tls.Certificate{}, err
	}

	caKey, err := certs.DecodePrivateKeyPEM(caKeyEncoded)
	if err != nil {
		return tls.Certificate{}, err
	}

	x509Cert, err := newClientCert(caCert, clientKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.X509KeyPair(certs.EncodeCertPEM(x509Cert), certs.EncodePrivateKeyPEM(clientKey))
}

func newClientCert(caCert *x509.Certificate, key *rsa.PrivateKey, caKey crypto.Signer) (*x509.Certificate, error) {
	cfg := certs.Config{
		CommonName: "cluster-api.x-k8s.io",
	}

	now := time.Now().UTC()

	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:   now.Add(time.Minute * -5),
		NotAfter:    now.Add(secret.TenYears), // 10 years
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	b, err := x509.CreateCertificate(rand.Reader, &tmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create signed client certificate: %+v", tmpl)
	}

	c, err := x509.ParseCertificate(b)

	return c, errors.WithStack(err)
}
