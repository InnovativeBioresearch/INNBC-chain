#!/usr/bin/env bash
# START ALPHA CHANGE 999 -- install standalone INNBC peer directory service package --
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
install -d /etc/systemd/system
# START ALPHA CHANGE 1921 -- create persistent peer directory state storage --
# START ALPHA CHANGE 1926 -- assign fixed INNBC runtime directories to the service account --
install -d -o innbc -g innbc -m 0750 /var/lib/innbc
install -d -o innbc -g innbc -m 0750 /var/lib/innbc/peer-directory
# FINISH ALPHA CHANGE 1926 -- assign fixed INNBC runtime directories to the service account --
# FINISH ALPHA CHANGE 1921 -- create persistent peer directory state storage --

install -m 0644 "${BUNDLE_DIR}/config/peerdirectory.env.example" /etc/innbc/peerdirectory.env.example
install -m 0644 "${BUNDLE_DIR}/systemd/innbc-peerdirectory.service" /etc/systemd/system/innbc-peerdirectory.service

if [ ! -f /etc/innbc/peerdirectory.env ]; then
  install -m 0600 "${BUNDLE_DIR}/config/peerdirectory.env.example" /etc/innbc/peerdirectory.env
  echo "Installed default peer directory env to /etc/innbc/peerdirectory.env"
else
  echo "Preserved existing /etc/innbc/peerdirectory.env"
fi

systemctl daemon-reload

echo "INNBC peer directory service package installed."
echo "This installer did not touch the node binary, genesis, node env files, validator keys, or chain data."
# FINISH ALPHA CHANGE 999 -- install standalone INNBC peer directory service package --