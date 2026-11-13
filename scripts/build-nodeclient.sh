#!/usr/bin/env bash
# START ALPHA CHANGE 745 -- build production INNBC nodeclient bundle --
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/dist/innbc-nodeclient"
PACKAGING_DIR="${ROOT_DIR}/packaging/nodeclient"
# START ALPHA CHANGE 999 -- define standalone peer directory service package output --
PEER_DIRECTORY_OUT_DIR="${ROOT_DIR}/dist/innbc-peerdirectory-service"
PEER_DIRECTORY_PACKAGING_DIR="${ROOT_DIR}/packaging/innbc-peerdirectory-service"
# FINISH ALPHA CHANGE 999 -- define standalone peer directory service package output --
# START ALPHA CHANGE 1845 -- define standalone snapshot builder package output --
SNAPSHOT_BUILDER_OUT_DIR="${ROOT_DIR}/dist/innbc-snapshot-builder"
SNAPSHOT_BUILDER_PACKAGING_DIR="${ROOT_DIR}/packaging/innbc-snapshot-builder"
# FINISH ALPHA CHANGE 1845 -- define standalone snapshot builder package output --

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required to build the INNBC nodeclient." >&2
  exit 1
fi

if [ ! -f "${ROOT_DIR}/genesis.json" ]; then
  echo "Missing ${ROOT_DIR}/genesis.json. Generate the canonical genesis before building the nodeclient." >&2
  exit 1
fi

rm -rf "${OUT_DIR}"
# START ALPHA CHANGE 999 -- refresh standalone peer directory service package output --
rm -rf "${PEER_DIRECTORY_OUT_DIR}"
# FINISH ALPHA CHANGE 999 -- refresh standalone peer directory service package output --
# START ALPHA CHANGE 1845 -- refresh standalone snapshot builder package output --
rm -rf "${SNAPSHOT_BUILDER_OUT_DIR}"
# FINISH ALPHA CHANGE 1845 -- refresh standalone snapshot builder package output --

mkdir -p \
  "${OUT_DIR}/bin" \
  "${OUT_DIR}/config" \
  "${OUT_DIR}/scripts" \
  "${OUT_DIR}/systemd"

VERSION="${VERSION:-local}"
COMMIT_HASH="${COMMIT_HASH:-none}"
BRANCH="${BRANCH:-manual}"
BUILD_TIME="$(date)"

go build -buildvcs=false -o "${OUT_DIR}/bin/innbc-chain" -ldflags="\
  -X 'github.com/innovativebioresearch/innbc-chain/versioning.Version=${VERSION}' \
  -X 'github.com/innovativebioresearch/innbc-chain/versioning.Commit=${COMMIT_HASH}' \
  -X 'github.com/innovativebioresearch/innbc-chain/versioning.Branch=${BRANCH}' \
  -X 'github.com/innovativebioresearch/innbc-chain/versioning.BuildTime=${BUILD_TIME}'" \
  "${ROOT_DIR}/main.go"

cp "${ROOT_DIR}/genesis.json" "${OUT_DIR}/config/genesis.json"
cp "${PACKAGING_DIR}/config/node.example.yaml" "${OUT_DIR}/config/node.example.yaml"
cp "${PACKAGING_DIR}/config/node.private-local.env.example" "${OUT_DIR}/config/node.private-local.env.example"
cp "${PACKAGING_DIR}/config/node.external-validator.env.example" "${OUT_DIR}/config/node.external-validator.env.example"
cp "${PACKAGING_DIR}/scripts/install.sh" "${OUT_DIR}/scripts/install.sh"
cp "${PACKAGING_DIR}/systemd/innbc-node@.service" "${OUT_DIR}/systemd/innbc-node@.service"
cp "${PACKAGING_DIR}/README-node-setup.md" "${OUT_DIR}/README-node-setup.md"

chmod +x "${OUT_DIR}/scripts/install.sh"

# START ALPHA CHANGE 999 -- copy standalone peer directory service package into dist --
mkdir -p "${PEER_DIRECTORY_OUT_DIR}"
cp -R "${PEER_DIRECTORY_PACKAGING_DIR}/." "${PEER_DIRECTORY_OUT_DIR}/"
chmod +x "${PEER_DIRECTORY_OUT_DIR}/scripts/install.sh"
# FINISH ALPHA CHANGE 999 -- copy standalone peer directory service package into dist --

# START ALPHA CHANGE 1845 -- copy standalone snapshot builder package into dist --
mkdir -p "${SNAPSHOT_BUILDER_OUT_DIR}"
cp -R "${SNAPSHOT_BUILDER_PACKAGING_DIR}/." "${SNAPSHOT_BUILDER_OUT_DIR}/"
chmod +x \
  "${SNAPSHOT_BUILDER_OUT_DIR}/scripts/install.sh" \
  "${SNAPSHOT_BUILDER_OUT_DIR}/scripts/innbc-snapshot-builder"
# FINISH ALPHA CHANGE 1845 -- copy standalone snapshot builder package into dist --

echo "INNBC nodeclient bundle created at:"
echo "${OUT_DIR}"
# START ALPHA CHANGE 999 -- print standalone peer directory service package output --
echo "INNBC peer directory service package created at:"
echo "${PEER_DIRECTORY_OUT_DIR}"
# FINISH ALPHA CHANGE 999 -- print standalone peer directory service package output --
# START ALPHA CHANGE 1845 -- print standalone snapshot builder package output --
echo "INNBC snapshot builder package created at:"
echo "${SNAPSHOT_BUILDER_OUT_DIR}"
# FINISH ALPHA CHANGE 1845 -- print standalone snapshot builder package output --
# FINISH ALPHA CHANGE 745 -- build production INNBC nodeclient bundle --