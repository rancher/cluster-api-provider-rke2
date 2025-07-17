# Configuring User Data Compression

## Overview

Cloud-init user-data can grow significantly in size, especially when many configurations and embedded scripts are used. Some infrastructure providers (e.g., AWS, OpenStack) have strict size limits for user-data, which can lead to provisioning failures. To address this, the gzipUserData field in RKE2ConfigSpec allows optional gzip compression of the user-data payload before it is passed to the infrastructure provider.

This feature helps reduce the size of the cloud-init payload but should only be used when the infrastructure provider supports gzipped user-data. For example, OpenStack typically handles it well, while others like Metal3 may not.

## Using Gzip Compression for User Data

The `gzipUserData` field in RKE2ConfigSpec allows you to enable gzip compression for the rendered cloud-init user data. 
By default, `gzipUserData` is set to `false`. When set to `true`, the user-data is gzipped before being passed to the infrastructure provider.

Example:

```yaml
apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
kind: RKE2Config
metadata:
  name: my-cluster-bootstrap
spec:
  gzipUserData: true
```
