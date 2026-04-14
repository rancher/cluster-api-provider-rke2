#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

# renovate: datasource=github-release-attachments depName=rancher/rke2
RKE2_VERSION="v1.26.0+rke2r1"

# Checksums for RKE2 ${RKE2_VERSION} amd64 artifacts.
# When updating RKE2_VERSION, update these checksums from the upstream
# sha256sum-amd64.txt for the new release before committing.
# renovate: datasource=github-release-attachments depName=rancher/rke2 digestVersion=v1.26.0+rke2r1
CHECKSUM_rke2_images_linux_amd64_tar_zst="9c71fc4280beaaebddf742dda51ef6337f3e0222ca9407356848bb8cb6b9acda"
# renovate: datasource=github-release-attachments depName=rancher/rke2 digestVersion=v1.26.0+rke2r1
CHECKSUM_rke2_linux_amd64_tar_gz="d79933aaadbe3435fa5b1ad4757c23d7055c4e0e6befe93781f21a93407e32fa"
# renovate: datasource=github-release-attachments depName=rancher/rke2 digestVersion=v1.26.0+rke2r1
CHECKSUM_sha256sum_amd64_txt="937a6a5dd0e926c173156cd8cad8d0e821c0aa651f5ce58dccfb094435432997"

# Map a filename to its expected checksum.
expected_checksum() {
  case "$1" in
    rke2-images.linux-amd64.tar.zst) echo "${CHECKSUM_rke2_images_linux_amd64_tar_zst}" ;;
    rke2.linux-amd64.tar.gz)         echo "${CHECKSUM_rke2_linux_amd64_tar_gz}" ;;
    sha256sum-amd64.txt)             echo "${CHECKSUM_sha256sum_amd64_txt}" ;;
    *) echo "ERROR: no checksum registered for artifact '$1'" >&2; exit 1 ;;
  esac
}

RKE2_RELEASE_BASE="https://github.com/rancher/rke2/releases/download/${RKE2_VERSION}"

echo "Downloading RKE2 artifacts for version ${RKE2_VERSION} ..."

mkdir -p files

while read -r p; do
  echo "Downloading $p ..."
  curl -sfL -o "files/$p" "${RKE2_RELEASE_BASE}/$p"
  # Verify against the locally-stored checksum — never trust a remotely-fetched
  # checksum file, as a compromised release could swap both the artifact and its
  # checksum simultaneously.
  EXPECTED="$(expected_checksum "$p")"
  echo "${EXPECTED}  files/$p" | sha256sum -c -
done <artifact-list.txt

echo "Done."
