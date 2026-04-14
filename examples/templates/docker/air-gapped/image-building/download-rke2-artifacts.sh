#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

: "${RKE2_VERSION:=v1.26.0+rke2r1}"

ARCH="${ARCH:-amd64}"
RKE2_RELEASE_BASE="https://github.com/rancher/rke2/releases/download/${RKE2_VERSION}"
CHECKSUM_FILE="sha256sum-${ARCH}.txt"

echo "Downloading RKE2 artifacts for version $RKE2_VERSION ..."

mkdir -p files

# Download the checksum file first so we can verify every artifact against it.
curl -sfL -o "files/${CHECKSUM_FILE}" "${RKE2_RELEASE_BASE}/${CHECKSUM_FILE}"

while read -r p; do
  echo "Downloading $p ..."
  curl -sfL -o "files/$p" "${RKE2_RELEASE_BASE}/$p"
  # Verify the downloaded file against the checksum.
  (cd files && grep " $p\$" "${CHECKSUM_FILE}" | sha256sum -c -)
done <artifact-list.txt

echo "Done."
