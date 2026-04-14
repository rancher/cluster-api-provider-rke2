#!/usr/bin/env bash

# Copyright © 2023 - 2024 SUSE LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

# shellcheck source=./hack/utils.sh
source "$(dirname "${BASH_SOURCE[0]}")/utils.sh"

GOPATH_BIN="$(go env GOPATH)/bin/"
MINIMUM_KUBECTL_VERSION=v1.27.0
goarch="$(go env GOARCH)"
goos="$(go env GOOS)"

# renovate-local: kubectl-linux-amd64=v1.27.0
KUBECTL_SUM_linux_amd64="71a78259d70da9c5540c4cf4cff121f443e863376f68f89a759d90cef3f51e87"
# renovate-local: kubectl-linux-arm64=v1.27.0
KUBECTL_SUM_linux_arm64="f8e09630211f2b7c6a8cc38835e7dea94708d401f5c84b23a37c70c604602ddc"
# renovate-local: kubectl-darwin-amd64=v1.27.0
KUBECTL_SUM_darwin_amd64="34aecd56036c71fd88b84bfbed3a984385927efbac091fe7d7ca99c431e0c00f"
# renovate-local: kubectl-darwin-arm64=v1.27.0
KUBECTL_SUM_darwin_arm64="f1dc2fdda8acb8abbc8e356f51082eef1b2d1f247c80a144d7bbed69a2b3c3b9"

# renovate: datasource=github-release-attachments depName=kubernetes-sigs/krew
KREW_VERSION="v0.5.0"
# renovate: datasource=github-release-attachments depName=kubernetes-sigs/krew digestVersion=v0.5.0
KREW_SUM_linux_amd64="5d5a221fffdf331d1c5c68d9917530ecd102e0def5b5a6d62eeed1c404efb28a"
# renovate: datasource=github-release-attachments depName=kubernetes-sigs/krew digestVersion=v0.5.0
KREW_SUM_linux_arm64="ab7a98b992424e76b6c162f8b67fb76c4b1e243598aa2807bdf226752f964548"
# renovate: datasource=github-release-attachments depName=kubernetes-sigs/krew digestVersion=v0.5.0
KREW_SUM_darwin_amd64="2d60559126452b57e3df0612f0475a473363f064da35f817290dbbcd877d1ea8"
# renovate: datasource=github-release-attachments depName=kubernetes-sigs/krew digestVersion=v0.5.0
KREW_SUM_darwin_arm64="cd6e58b4e954e301abd19001d772846997216d696bcaa58f0bcf04708339ece3"

# Ensure the kubectl tool exists and is a viable version, or installs it
verify_kubectl_version() {

  # If kubectl is not available on the path, get it
  if ! [ -x "$(command -v kubectl)" ]; then
    if [ "$goos" == "linux" ] || [ "$goos" == "darwin" ]; then
      if ! [ -d "${GOPATH_BIN}" ]; then
        mkdir -p "${GOPATH_BIN}"
      fi
      echo 'kubectl not found, installing'
      curl -sLo "${GOPATH_BIN}/kubectl" "https://dl.k8s.io/release/${MINIMUM_KUBECTL_VERSION}/bin/${goos}/${goarch}/kubectl"
      # Verify the downloaded binary against the known checksum.
      KUBECTL_SUM_VAR="KUBECTL_SUM_${goos}_${goarch}"
      echo "${!KUBECTL_SUM_VAR}  ${GOPATH_BIN}/kubectl" | sha256sum -c -
      chmod +x "${GOPATH_BIN}/kubectl"
      verify_gopath_bin
    else
      echo "Missing required binary in path: kubectl"
      return 2
    fi
  fi

  local kubectl_version
  IFS=" " read -ra kubectl_version <<< "$(kubectl version --client)"
  if [[ "${MINIMUM_KUBECTL_VERSION}" != $(echo -e "${MINIMUM_KUBECTL_VERSION}\n${kubectl_version[2]}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) ]]; then
    cat <<EOF
Detected kubectl version: ${kubectl_version[2]}.
Requires ${MINIMUM_KUBECTL_VERSION} or greater.
Please install ${MINIMUM_KUBECTL_VERSION} or later.
EOF
    return 2
  fi
}

install_plugins() {
  (
    set -x; cd "$(mktemp -d)" &&
    OS="$(uname | tr '[:upper:]' '[:lower:]')" &&
    ARCH="$(uname -m | sed -e 's/x86_64/amd64/' -e 's/\(arm\)\(64\)\?.*/\1\2/' -e 's/aarch64$/arm64/')" &&
    KREW="krew-${OS}_${ARCH}" &&
    curl -fsSLO "https://github.com/kubernetes-sigs/krew/releases/download/${KREW_VERSION}/${KREW}.tar.gz" &&
    # Verify the downloaded archive against the known checksum.
    KREW_SUM_VAR="KREW_SUM_${OS}_${ARCH}" &&
    echo "${!KREW_SUM_VAR}  ${KREW}.tar.gz" | sha256sum -c - &&
    tar zxvf "${KREW}.tar.gz" &&
    ./"${KREW}" install krew
  )
  kubectl krew install crust-gather
}

verify_kubectl_version
install_plugins
