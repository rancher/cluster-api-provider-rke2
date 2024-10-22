# Air-Gapped CAPD image building for CABPR

## Needed Artifacts for RKE2 Air Gapped Installation

In the folder [files](files/), you need to add the files `rke2.linux-amd64.tar.gz` and `rke2-images.linux-amd64.tar.zst` from the RKE2 target release. You can use the script [download-rke2-artifacts.sh](download-rke2-artifacts.sh) to download a specific version of RKE2 by setting the environment variable `RKE2_VERSION` to the desired version.

```bash
RKE2_VERSION=v1.23.16+rke2r1 ./download-rke2-artifacts.sh
```
which should, after successful download show:

```
Downloading RKE2 artifacts for version v1.23.16+rke2r1 ...
Done.
```

## Image Building

There is a [Dockerfile](Dockerfile) provided for reference in order to build a container image that works in Air-Gapped mode. After making sure the [files](files/) folder contains the RKE2 artifact files and the `install.sh` file, you can build a container image using the command:

```
export KUBERNETES_VERSION=v1.23.16
docker build -t rke2-ubuntu:$KUBERNETES_VERSION .
```

This image can then be uploaded to a container image registry and used in [manifest template for CAPD](../rke2-sample.yaml) under the `spec.template.spec.customImage` field for the `DockerMachineTemplate` object.
