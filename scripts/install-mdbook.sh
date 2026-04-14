#!/bin/bash

# Copyright 2022 The Kubernetes Authors.
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

VERSION=${1}
OUTPUT_PATH=${2}

# Ensure the output folder exists
mkdir -p "${OUTPUT_PATH}"

# Get what release to download
RELEASE_NAME=""
# renovate-local: mdbook-linux-x86_64=v0.4.40
MDBOOK_SUM_linux="9ef07fd288ba58ff3b99d1c94e6d414d431c9a61fdb20348e5beb74b823d546b"
# renovate-local: mdbook-darwin-x86_64=v0.4.40
MDBOOK_SUM_darwin="5783c09bb60b3e2e904d6839e3a1993a4ace1ca30a336a3c78bedac6e938817c"
MDBOOK_SUM=""
case "$OSTYPE" in
  darwin*) RELEASE_NAME="x86_64-apple-darwin.tar.gz"    ; MDBOOK_SUM="${MDBOOK_SUM_darwin}" ;;
  linux*)  RELEASE_NAME="x86_64-unknown-linux-gnu.tar.gz"; MDBOOK_SUM="${MDBOOK_SUM_linux}" ;;
#  msys*)    echo "WINDOWS" ;;
  *)        echo "No mdBook release available for: $OSTYPE" && exit 1;;
esac

TMPFILE="$(mktemp)"
trap 'rm -f "${TMPFILE}"' EXIT

# Download the mdBook release to a temporary file, verify the checksum, then extract.
curl -fsSL -o "${TMPFILE}" "https://github.com/rust-lang/mdBook/releases/download/${VERSION}/mdbook-${VERSION}-${RELEASE_NAME}"
echo "${MDBOOK_SUM}  ${TMPFILE}" | sha256sum -c -
tar -xvz -C "${OUTPUT_PATH}" -f "${TMPFILE}"
