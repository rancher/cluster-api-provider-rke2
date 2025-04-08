# Air-Gapped Cluster Deployment

## Introduction

The default way this provider uses to deploy RKE2 is by using the [online installation method](https://docs.rke2.io/install/quickstart). This method needs access to Rancher servers and Docker.io registry for downloading scripts, RKE2 packages and container images necessary to the installation of RKE2.

Some users might prefer using Air-Gapped installation for multiple possible reasons like deployment on particularly secure environments, sporadic access issues (like Deployment to Edge Locations) or Bandwidth preservation.

RKE2 supports Air-Gapped installation using :

- 2 methods for node preparation: Tarball on the node, Container Image Registry
  
- 2 methods for actual RKE2 installation after the node is prepared: Manual deployment, and Using `install.sh` from `https://get.rke2.io`.
  

## Methods supported by CAPRKE2 (Cluster API Provider RKE2)

In choosing between the RKE2 Air-Gapped cluster creation modes above, CAPRKE2 has chosen the best tradeoff in terms of simplicity, usability and limitation of dependencies.

### Node preparation

The method that is supported by CAPRKE2 is the Tarball on the node using custom images. The reasons behind this choice include:

- No dependency on the environments' network infrastructure and Image Registry, and the registry approach does not exempt from needing to use a custom image anyway.
  
- CAPI's philosophy is to accept custom-defined base images for infrastructure providers, which makes it easy to build the RKE2 pre-requisites (for a specific RKE2 version) into a custom image to be used for all deployments.
  

### RKE2 deployment

The method that is supported by CAPRKE2 for RKE2 deployment is by using the `install.sh` approach, described [here](https://docs.rke2.io/install/airgap#rke2-installsh-script-install). This approach is used because it automates a number of tasks needed for RKE2 to be deployed, like creating file hierarchy, unpacking Tarball, and creating `systemd` service units.

Since these tasks might change in the future, we prefer to rely on the upstream script from RKE2, available in the latest valid version at: https://get.rke2.io .

## Pre-requisites on base image

Considering the above tradeoffs, base images used for Air-Gapped need to comply to some pre-requisites in order to work with CAPRKE2. This sections list these pre-requisites:

- Support and presence of `cloud-init` (ignition bootstrapping is also on the roadmap)
  
- Presence of `systemd` (because RKE2's installation relies on systemd to start RKE2)
  
- Presence of the folders `/opt` and `/opt/rke2-artifacts` with the following files inside these folders:
  
  - `install.sh` in `/opt` (this file has the content of the script available at https://get.rke2.io ). One way to create it at build time is by using `curl -sfL https://get.rke2.io > /opt/install.sh` using a linux user with `write` permissions to the `/opt` folder.
    
  - `rke2-images.linux-amd64.tar.zst` , `rke2.linux-amd64.tar.gz` and `sha256sum-amd64.txt` in the `/opt/rke2-artifacts` folder, these files can be downloaded for a specific version of RKE2 on its release page, for instance, this page : [Release v1.23.16+rke2r1 · rancher/rke2 · GitHub](https://github.com/rancher/rke2/releases/tag/v1.23.16%2Brke2r1) for version `v1.23.16+rke2r1` . The files can be found under the Assets sections of the page.
    
- Previous pre-requisites should be built into an machine image, for instance, for instance a container image for CAPD or an AMI for AWS EC2. Each Infrastructure provider has its own way of defining machine images.
  

### Configuration of CAPRKE2 for Air-Gapped use

In order to deploy RKE2 Clusters in Air-Gapped mode using CAPRKE2, you need to set the fields `spec.agentConfig.airGapped` for the RKE2ControlPlane object and `spec.template.spec.agentConfig.airGapped` for RKE2ConfigTemplate object to `true`.

You can check a reference implementation for CAPD [here](https://github.com/rancher/cluster-api-provider-rke2/tree/main/examples/templates/docker/air-gapped) including configuration for CAPD custom image.
