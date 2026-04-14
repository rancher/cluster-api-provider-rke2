#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# renovate-local: awscli-exe-linux-x86_64=2.34.30
AWS_CLI_VERSION="2.34.30"
# renovate-local: awscli-exe-linux-x86_64=2.34.30
AWS_CLI_SUM="c78c02b818b14c5a2f745abc6752e73dcbd0bb5e65f10fb4363a48e9e720e2c0"

# RKE2 version and artifact checksums.
# When updating RKE2_EXPECTED_VERSION, also update the checksums below from the
# upstream sha256sum-amd64.txt for the new release before committing.
# renovate: datasource=github-release-attachments depName=rancher/rke2
RKE2_EXPECTED_VERSION="1.26.0+rke2r1"
# renovate: datasource=github-release-attachments depName=rancher/rke2 digestVersion=v1.26.0+rke2r1
RKE2_SUM_images="9c71fc4280beaaebddf742dda51ef6337f3e0222ca9407356848bb8cb6b9acda"
# renovate: datasource=github-release-attachments depName=rancher/rke2 digestVersion=v1.26.0+rke2r1
RKE2_SUM_tarball="d79933aaadbe3435fa5b1ad4757c23d7055c4e0e6befe93781f21a93407e32fa"

setup_infrastructure () {
  if [[ "$1" == "aws" ]]; then
    zypper --gpg-auto-import-keys --non-interactive install unzip amazon-ssm-agent
    curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64-${AWS_CLI_VERSION}.zip" -o "/tmp/awscliv2.zip"
    echo "${AWS_CLI_SUM}  /tmp/awscliv2.zip" | sha256sum -c -
    unzip /tmp/awscliv2.zip -d /tmp/
    /tmp/aws/install
    rm -rf /tmp/awscliv2.zip /tmp/aws
    systemctl enable amazon-ssm-agent
  fi
}

configure_systemd () {
  echo "Enabling required systemd services"

  systemctl enable sshd
  systemctl enable cloud-final
  systemctl enable cloud-config
  systemctl enable cloud-init
  systemctl enable cloud-init-local
  
  systemctl stop cloud-final
  systemctl stop cloud-config
  systemctl stop cloud-init
  systemctl stop cloud-init-local
}

configure_cloudinit () {
  echo "Configuring cloud-init"

  cloud-init clean -s -l
  rm -f /var/log/cloud-init*
  rm -rf /var/lib/cloud/*

  CLOUDINIT_PATH=$(python3 -c "import cloudinit; import os; print(os.path.dirname(cloudinit.__file__))")
  mv /tmp/features.py "${CLOUDINIT_PATH}/features.py"
}

cleanup_ssh_keys () {
  echo "Cleaning up SSH keys"

  rm -rf /etc/ssh/ssh_host_*
  rm -rf /root/.ssh/authorized_keys
  rm -rf /home/ec2-user/.ssh/authorized_keys
}

echo "Provisioning instance for $2"

echo "Install required packages"

zypper --gpg-auto-import-keys ref && \
zypper --gpg-auto-import-keys --non-interactive install \
        curl \
        openssh-server \
        cloud-init \
        systemd \
        openssh \

echo "Install RKE2 components"

# Validate that the requested version matches the pinned version whose
# checksums are stored in this script.  Update RKE2_EXPECTED_VERSION and the
# RKE2_SUM_* variables together whenever bumping the RKE2 release.
if [[ "${1}" != "${RKE2_EXPECTED_VERSION}" ]]; then
  echo "ERROR: requested RKE2 version '${1}' does not match the pinned version '${RKE2_EXPECTED_VERSION}'." >&2
  echo "Update RKE2_EXPECTED_VERSION and the RKE2_SUM_* checksums in this script." >&2
  exit 1
fi

mkdir -p /opt/rke2-artifacts
RKE2_RELEASE_BASE="https://github.com/rancher/rke2/releases/download/v${1}"

# Download each artifact and immediately verify against the locally-stored
# checksum — never trust a remotely-fetched checksum file.
curl -sfL -o /opt/rke2-artifacts/rke2-images.linux-amd64.tar.zst "${RKE2_RELEASE_BASE}/rke2-images.linux-amd64.tar.zst"
echo "${RKE2_SUM_images}  /opt/rke2-artifacts/rke2-images.linux-amd64.tar.zst" | sha256sum -c -
curl -sfL -o /opt/rke2-artifacts/rke2.linux-amd64.tar.gz "${RKE2_RELEASE_BASE}/rke2.linux-amd64.tar.gz"
echo "${RKE2_SUM_tarball}  /opt/rke2-artifacts/rke2.linux-amd64.tar.gz" | sha256sum -c -
curl -sfL -o /opt/install.sh https://get.rke2.io

configure_systemd 
configure_cloudinit
cleanup_ssh_keys
setup_infrastructure $2

echo "Done"