#!/usr/bin/env bash
# START ALPHA CHANGE 1848 -- install standalone snapshot builder package --
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

install -d /etc/innbc
install -d /usr/local/bin
# START ALPHA CHANGE 1926 -- assign fixed INNBC runtime directories to the service account --
install -d -o innbc -g innbc -m 0750 /var/lib/innbc
install -d -o innbc -g innbc -m 0750 /var/lib/innbc/snapshots
# START ALPHA CHANGE 2083 -- create the historical archive publication root --
install -d -o innbc -g innbc -m 0750 /var/lib/innbc/history-archive
# START ALPHA CHANGE 2278 -- prepare the independent database backup output root --
install -d -o innbc -g innbc -m 0750 /var/lib/innbc/database-backups
# FINISH ALPHA CHANGE 2278 -- prepare the independent database backup output root --
# FINISH ALPHA CHANGE 2083 -- create the historical archive publication root --
# FINISH ALPHA CHANGE 1926 -- assign fixed INNBC runtime directories to the service account --

install -m 0644 \
  "${BUNDLE_DIR}/config/snapshot-builder.env.example" \
  /etc/innbc/snapshot-builder.env.example

install -m 0755 \
  "${BUNDLE_DIR}/scripts/innbc-snapshot-builder" \
  /usr/local/bin/innbc-snapshot-builder

if [ ! -f /etc/innbc/snapshot-builder.env ]; then
  install -m 0600 \
    "${BUNDLE_DIR}/config/snapshot-builder.env.example" \
    /etc/innbc/snapshot-builder.env

  echo "Installed default snapshot builder env to /etc/innbc/snapshot-builder.env"
else
  echo "Preserved existing /etc/innbc/snapshot-builder.env"
fi

echo "INNBC snapshot builder package installed."
echo "Run: sudo innbc-snapshot-builder segmented"
echo "Or:  sudo innbc-snapshot-builder checkpoint"
# START ALPHA CHANGE 2083 -- report archive-aware package commands --
echo "Or:  sudo innbc-snapshot-builder archive"
echo "Or:  sudo innbc-snapshot-builder recovery-checkpoint"
# START ALPHA CHANGE 2278 -- report the environment-driven backup command --
echo "Or:  sudo innbc-snapshot-builder backup"
# FINISH ALPHA CHANGE 2278 -- report the environment-driven backup command --
# FINISH ALPHA CHANGE 2083 -- report archive-aware package commands --
echo "This installer did not touch the node binary, genesis, node env files, validator keys, chain data, or existing snapshots."
# FINISH ALPHA CHANGE 1848 -- install standalone snapshot builder package --