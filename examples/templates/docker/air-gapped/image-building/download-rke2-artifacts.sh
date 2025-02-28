#!/bin/bash
: "${RKE2_VERSION:=v1.26.0+rke2r1}"
echo "Downloading RKE2 artifacts for version $RKE2_VERSION ..."
while read p; do
  curl -sfL -o files/$p https://github.com/rancher/rke2/releases/download/$RKE2_VERSION/$p
done <artifact-list.txt
echo "Done."
