#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

install_awspackages () {
  if [[ "$1" != "aws" ]]; then
    return
  fi
  zypper --gpg-auto-import-keys --non-interactive install awscli amazon-ssm-agent
}

configure_systemd () {
  echo "Enabling required systemd services"

  if [[ "$1" == "aws" ]]; then
    systemctl enable amazon-ssm-agent
    systemctl start amazon-ssm-agent
  fi

  systemctl enable sshd
  systemctl enable cloud-final
  systemctl enable cloud-config
  systemctl enable cloud-init
  systemctl enable cloud-init-local
  systemctl start sshd
  systemctl start cloud-final
  systemctl start cloud-config
  systemctl start cloud-init
  systemctl start cloud-init-local
}

configure_cloudinit () {
  echo "Configuring cloud-init"

  cloud-init clean
  rm -f /var/log/cloud-init*

  mkdir -p /usr/lib/python3/dist-packages/cloudinit
  mv /tmp/feature_overrides.py /usr/lib/python3/dist-packages/cloudinit/feature_overrides.py
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

install_awspackages $2

configure_systemd $2 
configure_cloudinit $2

echo "Install RKE2 components"

mkdir -p /opt/rke2-artifacts
curl -sfL -o /opt/rke2-artifacts/rke2-images.linux-amd64.tar.zst https://github.com/rancher/rke2/releases/download/v${1}/rke2-images.linux-amd64.tar.zst
curl -sfL -o /opt/rke2-artifacts/rke2.linux-amd64.tar.gz https://github.com/rancher/rke2/releases/download/v${1}/rke2.linux-amd64.tar.gz
curl -sfL -o /opt/rke2-artifacts/sha256sum-amd64.txt https://github.com/rancher/rke2/releases/download/v${1}/sha256sum-amd64.txt
curl -sfL -o /opt/install.sh https://get.rke2.io

echo "Done"
