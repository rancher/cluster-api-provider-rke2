#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail


if [[ "$1" == "opensuse-1504" ]]; then
DOCKER_IMAGE=ghcr.io/rancher-sandbox/opensuse-15.04-rke2
fi
if [[ "$1" == "ubuntu-2204" ]]; then
DOCKER_IMAGE=ghcr.io/rancher-sandbox/ubuntu-22.04-rke2
fi

TRIMMED_RKE2_VERSION=v$(printf "$2" | sed 's/+rke2r1//g')

docker build . -f docker/$1/Dockerfile --build-arg RKE2_VERSION=$2 -t ${DOCKER_IMAGE}:$TRIMMED_RKE2_VERSION
docker push ${DOCKER_IMAGE}:$TRIMMED_RKE2_VERSION
