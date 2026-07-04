#!/usr/bin/env bash
#
# Package the (already built) encapi-cli binary into an RPM with nfpm.
# Usage: scripts/build-rpm.sh [version]   (version defaults to $CI_COMMIT_TAG)
#
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

VERSION="${1:-${CI_COMMIT_TAG:-0.0.0-dev}}"
VERSION="${VERSION#v}" # strip a leading v
DIST="dist"

if [ ! -f "${DIST}/encapi-cli" ]; then
  echo "ERROR: ${DIST}/encapi-cli not found; run 'make build-cli' first" >&2
  exit 1
fi

export PACKAGE_NAME="encapi-cli"
export PACKAGE_VERSION="${VERSION}"
export PACKAGE_RELEASE="1"
export PACKAGE_ARCH="amd64"
export PACKAGE_PLATFORM="linux"
export PACKAGE_DESCRIPTION="CLI + Puppet ENC entrypoint for encapi (External Node Classifier)"
export PACKAGE_MAINTAINER="Ben Vincent <12552114+neoloc@users.noreply.github.com>"
export PACKAGE_HOMEPAGE="https://github.com/IUCLabs/encapi"
export PACKAGE_LICENSE="MIT"

envsubst < packaging/nfpm.yaml > "${DIST}/nfpm.yaml"
nfpm pkg --config "${DIST}/nfpm.yaml" --target "${DIST}" --packager rpm

echo "Built:"
ls -1 "${DIST}"/*.rpm
