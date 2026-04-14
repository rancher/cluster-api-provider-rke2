#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# renovate-local: awscli-exe-linux-x86_64=2.34.30
AWS_CLI_VERSION="2.34.30"
# renovate-local: awscli-exe-linux-x86_64=2.34.30
AWS_CLI_SUM="c78c02b818b14c5a2f745abc6752e73dcbd0bb5e65f10fb4363a48e9e720e2c0"

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

mkdir -p /opt/rke2-artifacts
RKE2_RELEASE_BASE="https://github.com/rancher/rke2/releases/download/v${1}"

# Download checksum file first, then verify each artifact against it.
curl -sfL -o /opt/rke2-artifacts/sha256sum-amd64.txt "${RKE2_RELEASE_BASE}/sha256sum-amd64.txt"
curl -sfL -o /opt/rke2-artifacts/rke2-images.linux-amd64.tar.zst "${RKE2_RELEASE_BASE}/rke2-images.linux-amd64.tar.zst"
(cd /opt/rke2-artifacts && grep " rke2-images.linux-amd64.tar.zst$" sha256sum-amd64.txt | sha256sum -c -)
curl -sfL -o /opt/rke2-artifacts/rke2.linux-amd64.tar.gz "${RKE2_RELEASE_BASE}/rke2.linux-amd64.tar.gz"
(cd /opt/rke2-artifacts && grep " rke2.linux-amd64.tar.gz$" sha256sum-amd64.txt | sha256sum -c -)
curl -sfL -o /opt/install.sh https://get.rke2.io

configure_systemd 
configure_cloudinit
cleanup_ssh_keys
setup_infrastructure $2

echo "Done"