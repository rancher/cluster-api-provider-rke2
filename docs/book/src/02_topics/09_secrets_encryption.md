# Configuring Secrets encryption 

## Overview

By default, RKE2 enables Secrets encryption at rest with `aescbc` provider and generates private key automatically. [Reference](https://docs.rke2.io/security/secrets_encryption)

## Customizing Encryption provider

To configure different provider (`aescbc` or `secretbox`) or specify encryption key explicitly, configure `spec.serverConfig.secretsEncryption` block.

Example:

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: RKE2ControlPlane
metadata:
  name: my-cluster-control-plane
spec:
  serverConfig:
    secretsEncryption:
      provider: "secretbox"
      encryptionKeySecret:
        name: encryption-key
        namespace: example
```

## Encryption secret format

When configuring the `encryptionKeySecret` field, ensure the secret contains the following keys:

- **encryptionKey** - base64 decoded value of the encryption key
