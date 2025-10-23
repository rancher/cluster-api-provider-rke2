# Configuring Secrets encryption 

## Overview

By default, RKE2 enables Secret encryotion at rest with `aescbc` provider and generate private key automatically. [Refer](https://docs.rke2.io/security/secrets_encryption)

## Customizing Encryption provider

To configure different provider (`aescbc` or `secretbox`) or specify encryption key explicitly configure `spec.serverConfig.secretsEncryption` block

Expample:

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
        namespace: exmaple
```

## Encryption secret format

When configuring the `encryptionKeySecret`, ensure the secret contains the following keys:

- **encryptionKey** - base64 decoded value of the encryption key
