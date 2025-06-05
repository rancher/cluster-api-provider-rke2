# Configuring Embedded Registry in RKE2

## Overview

RKE2 allows users to enable an **embedded registry** on control plane nodes. When the `embeddedRegistry` option is set to `true` in the `serverConfig`, users can configure the registry using the `PrivateRegistriesConfig` field.
The process follows [RKE2 docs](https://docs.rke2.io/install/registry_mirror).

## Enabling Embedded Registry

To enable the embedded registry, set the `embeddedRegistry` field to `true` in the `serverConfig` section of the `RKE2ControlPlane` configuration:

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: RKE2ControlPlane
metadata:
  name: my-cluster-control-plane
spec:
  serverConfig:
    embeddedRegistry: true
```

## Configuring Private Registries

Once the embedded registry is enabled, you can configure private registries using the `PrivateRegistriesConfig` field in `RKE2ConfigSpec`. This field allows you to define registry mirrors, authentication, and TLS settings.

Example:

```yaml
apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
kind: RKE2Config
metadata:
  name: my-cluster-bootstrap
spec:
  privateRegistriesConfig:
    mirrors:
      "myregistry.example.com":
        endpoint:
          - "https://mirror1.example.com"
          - "https://mirror2.example.com"
    configs:
      "myregistry.example.com":
        authSecret:
          name: my-registry-secret
          namespace: my-secrets-namespace
        tls:
          tlsConfigSecret:
            name: my-registry-tls-secret
            namespace: my-secrets-namespace
          insecureSkipVerify: false
```

## TLS Secret Format

When configuring the `tlsConfigSecret`, ensure the secret contains the following keys:

- **`ca.crt`** – CA certificate
- **`tls.key`** – TLS private key
- **`tls.crt`** – TLS certificate

## Auth Secret Format

When configuring the `authSecret`, ensure the secret contains the following keys:

- **`username` and `password`** - When using Basic Auth credentials
- **`identity-token`** - When using a personal access token
