#!/usr/bin/env bash
# START ALPHA CHANGE 746 -- install production INNBC nodeclient bundle without touching validator identities --
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "This installer must be run as root." >&2
  exit 1
fi

# START ALPHA CHANGE 1925 -- create one non-login INNBC runtime account --
INNBC_RUNTIME_USER="innbc"
INNBC_RUNTIME_GROUP="innbc"

ensure_innbc_runtime_account() {
  if ! getent group "${INNBC_RUNTIME_GROUP}" >/dev/null; then
    groupadd --system "${INNBC_RUNTIME_GROUP}"
  fi

  if getent passwd "${INNBC_RUNTIME_USER}" >/dev/null; then
    local primary_group
    local login_shell

    primary_group="$(id -gn "${INNBC_RUNTIME_USER}")"
    login_shell="$(getent passwd "${INNBC_RUNTIME_USER}" | cut -d: -f7)"

    if [ "${primary_group}" != "${INNBC_RUNTIME_GROUP}" ]; then
      echo "Existing ${INNBC_RUNTIME_USER} account does not use primary group ${INNBC_RUNTIME_GROUP}." >&2
      exit 1
    fi

    case "${login_shell}" in
      /usr/sbin/nologin|/sbin/nologin|/bin/false)
        ;;
      *)
        echo "Existing ${INNBC_RUNTIME_USER} account has an interactive login shell: ${login_shell}" >&2
        exit 1
        ;;
    esac

    return
  fi

  useradd \
    --system \
    --gid "${INNBC_RUNTIME_GROUP}" \
    --home-dir /var/lib/innbc \
    --no-create-home \
    --shell /usr/sbin/nologin \
    --comment "INNBC service account" \
    "${INNBC_RUNTIME_USER}"
}

ensure_innbc_runtime_account
# FINISH ALPHA CHANGE 1925 -- create one non-login INNBC runtime account --

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUNDLE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

install -d /usr/local/bin
install -d /etc/innbc
# START ALPHA CHANGE 1926 -- assign fixed INNBC runtime directories to the service account --
install -d -o innbc -g innbc -m 0750 /var/lib/innbc
install -d -o innbc -g innbc -m 0750 /var/log/innbc
# FINISH ALPHA CHANGE 1926 -- assign fixed INNBC runtime directories to the service account --
install -d /etc/systemd/system

install -m 0755 "${BUNDLE_DIR}/bin/innbc-chain" /usr/local/bin/innbc-chain
install -m 0644 "${BUNDLE_DIR}/config/node.example.yaml" /etc/innbc/node.example.yaml
install -m 0644 "${BUNDLE_DIR}/config/node.private-local.env.example" /etc/innbc/node.private-local.env.example
install -m 0644 "${BUNDLE_DIR}/config/node.external-validator.env.example" /etc/innbc/node.external-validator.env.example
install -m 0644 "${BUNDLE_DIR}/systemd/innbc-node@.service" /etc/systemd/system/innbc-node@.service

if [ ! -f /etc/innbc/genesis.json ]; then
  install -m 0644 "${BUNDLE_DIR}/config/genesis.json" /etc/innbc/genesis.json
  echo "Installed packaged genesis to /etc/innbc/genesis.json"
else
  echo "Preserved existing /etc/innbc/genesis.json"
fi

systemctl daemon-reload

echo "INNBC nodeclient installed."
echo "Validator identity folders are not created by this installer."
# FINISH ALPHA CHANGE 746 -- install production INNBC nodeclient bundle without touching validator identities --