#!/usr/bin/env bash
# START ALPHA CHANGE 2090 -- exercise packaged dispatch using isolated temporary commands --
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACKAGE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
WRAPPER="${SCRIPT_DIR}/innbc-snapshot-builder"
INSTALLER="${SCRIPT_DIR}/install.sh"
ENV_EXAMPLE="${PACKAGE_DIR}/config/snapshot-builder.env.example"

bash -n "${WRAPPER}"
bash -n "${INSTALLER}"
bash -n "${BASH_SOURCE[0]}"

TEST_ROOT="$(mktemp -d)"
trap 'rm -rf -- "${TEST_ROOT}"' EXIT

STUB_DIR="${TEST_ROOT}/stub-bin"
CASE_DIR="${TEST_ROOT}/paths with spaces"
TEST_WRAPPER="${TEST_ROOT}/wrapper"
TEST_CONFIG="${CASE_DIR}/builder.env"
TEST_BINARY="${CASE_DIR}/innbc-chain"
ARGUMENTS_FILE="${TEST_ROOT}/arguments"
RUNUSER_FILE="${TEST_ROOT}/runuser-arguments"

mkdir -p "${STUB_DIR}" "${CASE_DIR}"

# Preserve wrapper behavior while redirecting its two fixed deployment paths.
awk '
  $0 == "CONFIG_PATH=\"/etc/innbc/snapshot-builder.env\"" {
    print "CONFIG_PATH=\"${INNBC_WRAPPER_TEST_CONFIG}\""
    config_count++
    next
  }
  $0 == "INNBC_BINARY=\"/usr/local/bin/innbc-chain\"" {
    print "INNBC_BINARY=\"${INNBC_WRAPPER_TEST_BINARY}\""
    binary_count++
    next
  }
  { print }
  END {
    if (config_count != 1 || binary_count != 1) {
      exit 1
    }
  }
' "${WRAPPER}" > "${TEST_WRAPPER}"

cat > "${STUB_DIR}/id" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  "-u"|"-u innbc")
    echo 0
    ;;
  *)
    exit 91
    ;;
esac
STUB

cat > "${STUB_DIR}/runuser" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\0' "$@" > "${INNBC_WRAPPER_TEST_RUNUSER}"
if [ "$#" -lt 4 ] ||
   [ "$1" != "--user" ] ||
   [ "$2" != "innbc" ] ||
   [ "$3" != "--" ]; then
  exit 92
fi
shift 3
exec "$@"
STUB

cat > "${TEST_BINARY}" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\0' "$@" > "${INNBC_WRAPPER_TEST_ARGUMENTS}"
exit "${INNBC_WRAPPER_TEST_STATUS}"
STUB

chmod 0700 "${STUB_DIR}/id" "${STUB_DIR}/runuser" "${TEST_BINARY}"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

setting() {
  printf '%s=%q\n' "$1" "$2" >> "${TEST_CONFIG}"
}

# START ALPHA CHANGE 3079 -- exercise the intended launch storage configuration --
common_config() {
  : > "${TEST_CONFIG}"
  setting INNBC_SNAPSHOT_BUILDER_CHAIN "${CASE_DIR}/genesis.json"
  setting INNBC_SNAPSHOT_BUILDER_SOURCE_DATA_DIR "${CASE_DIR}/source node"
  setting INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB 1024
}

ordinary_config() {
  setting INNBC_SNAPSHOT_BUILDER_OUTPUT_ROOT "${CASE_DIR}/snapshots"
  setting INNBC_SNAPSHOT_BUILDER_RETENTION_BLOCKS 43200
  setting INNBC_SNAPSHOT_BUILDER_SEGMENT_BLOCKS 1800
  setting INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB 128
}
# FINISH ALPHA CHANGE 3079 -- exercise the intended launch storage configuration --

archive_config() {
  setting INNBC_HISTORY_ARCHIVE_OUTPUT_ROOT "${CASE_DIR}/archive"
  setting INNBC_HISTORY_ARCHIVE_SEGMENT_BLOCKS 1800
  setting INNBC_HISTORY_ARCHIVE_MAX_INDEX_RECORDS 2000000
}

recovery_config() {
  setting INNBC_SNAPSHOT_BUILDER_OUTPUT_ROOT "${CASE_DIR}/snapshots"
  setting INNBC_HISTORY_ARCHIVE_OUTPUT_ROOT "${CASE_DIR}/archive"
  setting INNBC_RECOVERY_CHECKPOINT_MINIMUM_RETENTION_BLOCKS 43200
}

CHILD_STATUS=0

run_case() {
  local expected_status="$1"
  shift
  local actual_status=0

  rm -f -- "${ARGUMENTS_FILE}" "${RUNUSER_FILE}"

  /usr/bin/env -i \
    PATH="${STUB_DIR}:/usr/bin:/bin" \
    INNBC_WRAPPER_TEST_CONFIG="${TEST_CONFIG}" \
    INNBC_WRAPPER_TEST_BINARY="${TEST_BINARY}" \
    INNBC_WRAPPER_TEST_ARGUMENTS="${ARGUMENTS_FILE}" \
    INNBC_WRAPPER_TEST_RUNUSER="${RUNUSER_FILE}" \
    INNBC_WRAPPER_TEST_STATUS="${CHILD_STATUS}" \
    /bin/bash "${TEST_WRAPPER}" "$@" \
    > "${TEST_ROOT}/stdout" \
    2> "${TEST_ROOT}/stderr" || actual_status="$?"

  if [ "${actual_status}" -ne "${expected_status}" ]; then
    cat "${TEST_ROOT}/stderr" >&2
    fail "expected status ${expected_status}, got ${actual_status}: $*"
  fi
}

assert_arguments() {
  local expected=("$@")
  local actual=()
  local runuser_arguments=()
  local index

  [ -f "${ARGUMENTS_FILE}" ] || fail "node command was not invoked"
  [ -f "${RUNUSER_FILE}" ] || fail "runuser was not invoked"

  mapfile -d '' -t actual < "${ARGUMENTS_FILE}"
  mapfile -d '' -t runuser_arguments < "${RUNUSER_FILE}"

  [ "${#actual[@]}" -eq "${#expected[@]}" ] ||
    fail "argument count differs"

  for index in "${!expected[@]}"; do
    [ "${actual[index]}" = "${expected[index]}" ] ||
      fail "argument ${index}: expected '${expected[index]}', got '${actual[index]}'"
  done

  [ "${#runuser_arguments[@]}" -eq "$(( ${#expected[@]} + 4 ))" ] ||
    fail "runuser argument count differs"
  [ "${runuser_arguments[0]}" = "--user" ] || fail "missing --user"
  [ "${runuser_arguments[1]}" = "innbc" ] || fail "wrong runtime user"
  [ "${runuser_arguments[2]}" = "--" ] || fail "missing runuser separator"
  [ "${runuser_arguments[3]}" = "${TEST_BINARY}" ] || fail "wrong executable"
}

assert_not_invoked() {
  [ ! -e "${ARGUMENTS_FILE}" ] || fail "node unexpectedly invoked"
  [ ! -e "${RUNUSER_FILE}" ] || fail "runuser unexpectedly invoked"
}

# START ALPHA CHANGE 3079 -- verify snapshot allowances follow their consumer roles --
common_config
ordinary_config

for kind in segmented checkpoint; do
  expected_map_size=1024
  if [ "${kind}" = segmented ]; then
    expected_map_size=128
  fi

  run_case 0 "${kind}"
  assert_arguments \
    snapshot build "${kind}" \
    --chain "${CASE_DIR}/genesis.json" \
    --source-data-dir "${CASE_DIR}/source node" \
    --output-root "${CASE_DIR}/snapshots" \
    --retention-blocks 43200 \
    --segment-blocks 1800 \
    --mdbx-page-size-bytes=4096 \
    --mdbx-map-size-gb "${expected_map_size}"
  # START ALPHA CHANGE 3482 -- assert default snapshot destination pages --
  # FINISH ALPHA CHANGE 3482 -- assert default snapshot destination pages --
done
# FINISH ALPHA CHANGE 3079 -- verify snapshot allowances follow their consumer roles --

run_case 1 archive
assert_not_invoked
grep -Fq 'INNBC_HISTORY_ARCHIVE_OUTPUT_ROOT' "${TEST_ROOT}/stderr" ||
  fail "missing archive setting was not identified"

run_case 1 recovery-checkpoint
assert_not_invoked

# Archive must not require ordinary snapshot or recovery-only settings.
common_config
archive_config

# START ALPHA CHANGE 3079 -- verify historical archive dispatch uses one TiB --
run_case 0 archive
assert_arguments \
  snapshot archive \
  --chain "${CASE_DIR}/genesis.json" \
  --source-data-dir "${CASE_DIR}/source node" \
  --output-root "${CASE_DIR}/archive" \
  --segment-blocks 1800 \
  --max-index-records 2000000 \
  --mdbx-page-size-bytes=4096 \
  --mdbx-map-size-gb 1024
# START ALPHA CHANGE 3482 -- assert default archive destination pages --
# FINISH ALPHA CHANGE 3482 -- assert default archive destination pages --
# FINISH ALPHA CHANGE 3079 -- verify historical archive dispatch uses one TiB --

# Recovery must not require archive segment/index construction settings.
common_config
recovery_config

# START ALPHA CHANGE 3079 -- verify recovery checkpoint dispatch uses one TiB --
run_case 0 recovery-checkpoint
assert_arguments \
  snapshot recovery-checkpoint \
  --chain "${CASE_DIR}/genesis.json" \
  --source-data-dir "${CASE_DIR}/source node" \
  --snapshot-output-root "${CASE_DIR}/snapshots" \
  --archive-output-root "${CASE_DIR}/archive" \
  --minimum-retention-blocks 43200 \
  --mdbx-page-size-bytes=4096 \
  --mdbx-map-size-gb 1024
# START ALPHA CHANGE 3482 -- assert default recovery destination pages --
# FINISH ALPHA CHANGE 3482 -- assert default recovery destination pages --
# FINISH ALPHA CHANGE 3079 -- verify recovery checkpoint dispatch uses one TiB --

# START ALPHA CHANGE 2279 -- verify backup dispatch configuration and privilege dropping --
# A missing backup setting must fail before starting the node binary.
common_config
run_case 1 backup
assert_not_invoked
grep -Fq 'INNBC_DATABASE_BACKUP_OUTPUT_ROOT' "${TEST_ROOT}/stderr" ||
  fail "missing backup output setting was not identified"

# Backup needs only common settings and its own output root.
# Paths with spaces must survive as individual arguments.
setting INNBC_DATABASE_BACKUP_OUTPUT_ROOT "${CASE_DIR}/database backups"

# START ALPHA CHANGE 3079 -- verify full-history database backup dispatch uses one TiB --
run_case 0 backup
assert_arguments \
  snapshot backup \
  --chain "${CASE_DIR}/genesis.json" \
  --data-dir "${CASE_DIR}/source node" \
  --out "${CASE_DIR}/database backups" \
  --mdbx-page-size-bytes=4096 \
  --mdbx-map-size-gb 1024
# START ALPHA CHANGE 3482 -- assert default backup destination pages --
# FINISH ALPHA CHANGE 3482 -- assert default backup destination pages --
# FINISH ALPHA CHANGE 3079 -- verify full-history database backup dispatch uses one TiB --

grep -Fq \
  'INNBC_DATABASE_BACKUP_OUTPUT_ROOT=/var/lib/innbc/database-backups' \
  "${ENV_EXAMPLE}" ||
  fail "missing database backup environment example"

grep -Fq '/var/lib/innbc/database-backups' "${INSTALLER}" ||
  fail "installer does not declare database backup storage"
# FINISH ALPHA CHANGE 2279 -- verify backup dispatch configuration and privilege dropping --

# Each branch must propagate the process status returned by the node.
common_config
ordinary_config
archive_config
setting INNBC_RECOVERY_CHECKPOINT_MINIMUM_RETENTION_BLOCKS 43200
# START ALPHA CHANGE 2279 -- include backup in child failure propagation --
setting INNBC_DATABASE_BACKUP_OUTPUT_ROOT "${CASE_DIR}/database backups"
CHILD_STATUS=37

for operation in segmented checkpoint archive recovery-checkpoint backup; do
# FINISH ALPHA CHANGE 2279 -- include backup in child failure propagation --
  run_case 37 "${operation}"
  [ -f "${ARGUMENTS_FILE}" ] || fail "failure-status case did not invoke node"
done
CHILD_STATUS=0

run_case 2 unsupported
assert_not_invoked
run_case 2
assert_not_invoked
run_case 2 archive extra-argument
assert_not_invoked

# Missing common configuration must stop dispatch before privilege dropping.
: > "${TEST_CONFIG}"
run_case 1 archive
assert_not_invoked
grep -Fq 'INNBC_SNAPSHOT_BUILDER_CHAIN' "${TEST_ROOT}/stderr" ||
  fail "missing common setting was not identified"

# Retain small static checks for packaging declarations.
for text in \
  'INNBC_HISTORY_ARCHIVE_OUTPUT_ROOT=/var/lib/innbc/history-archive' \
  'INNBC_HISTORY_ARCHIVE_SEGMENT_BLOCKS=1800' \
  'INNBC_HISTORY_ARCHIVE_MAX_INDEX_RECORDS=2000000' \
  'INNBC_RECOVERY_CHECKPOINT_MINIMUM_RETENTION_BLOCKS=43200'; do
  grep -Fq -- "${text}" "${ENV_EXAMPLE}" ||
    fail "missing environment example entry: ${text}"
done

grep -Fq '/var/lib/innbc/history-archive' "${INSTALLER}" ||
  fail "installer does not declare archive storage"

if grep -Fq 'INNBC_RECOVERY_SET_OUTPUT_ROOT' "${WRAPPER}" "${ENV_EXAMPLE}"; then
  fail "obsolete recovery-set output root returned"
fi

# START ALPHA CHANGE 3068 -- verify full-history and recent builder allowances independently --
assert_map_size() {
  local expected="$1"
  local actual=()
  local argument_count

  [ -f "${ARGUMENTS_FILE}" ] || fail "node command was not invoked"
  mapfile -d '' -t actual < "${ARGUMENTS_FILE}"
  argument_count="${#actual[@]}"

  [ "${argument_count}" -ge 2 ] || fail "missing MDBX arguments"
  [ "${actual[argument_count-2]}" = "--mdbx-map-size-gb" ] ||
    fail "missing destination MDBX flag"
  [ "${actual[argument_count-1]}" = "${expected}" ] ||
    fail "expected MDBX allowance ${expected}, got ${actual[argument_count-1]}"
}

common_config
ordinary_config
archive_config
setting INNBC_RECOVERY_CHECKPOINT_MINIMUM_RETENTION_BLOCKS 43200
setting INNBC_DATABASE_BACKUP_OUTPUT_ROOT "${CASE_DIR}/database backups"

# START ALPHA CHANGE 3079 -- verify explicitly configured launch allowances for every operation --
for operation in segmented checkpoint archive recovery-checkpoint backup; do
# FINISH ALPHA CHANGE 3079 -- verify explicitly configured launch allowances for every operation --
  run_case 0 "${operation}"
  if [ "${operation}" = segmented ]; then
    assert_map_size 128
  else
    assert_map_size 1024
  fi
done

# START ALPHA CHANGE 3482 -- verify explicit and blank page settings for every operation --
for page_setting in 8192 ""; do
  setting INNBC_SNAPSHOT_BUILDER_MDBX_PAGE_SIZE_BYTES "${page_setting}"
  expected_page="${page_setting:-4096}"

  for operation in segmented checkpoint archive recovery-checkpoint backup; do
    run_case 0 "${operation}"
    mapfile -d '' -t page_arguments < "${ARGUMENTS_FILE}"
    found_page=false
    for argument in "${page_arguments[@]}"; do
      if [ "${argument}" = "--mdbx-page-size-bytes=${expected_page}" ]; then
        found_page=true
      fi
    done
    [ "${found_page}" = true ] ||
      fail "missing page setting ${expected_page} for ${operation}"
  done
done
# FINISH ALPHA CHANGE 3482 -- verify explicit and blank page settings for every operation --

# An explicit recent allowance affects only segmented snapshots.
setting INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB 64
run_case 0 segmented
assert_map_size 64
run_case 0 checkpoint
assert_map_size 1024
run_case 0 backup
assert_map_size 1024

setting INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB 128
run_case 0 segmented
assert_map_size 128

# START ALPHA CHANGE 3079 -- reject missing or empty segmented allowances before dispatch --
common_config
setting INNBC_SNAPSHOT_BUILDER_OUTPUT_ROOT "${CASE_DIR}/snapshots"
setting INNBC_SNAPSHOT_BUILDER_RETENTION_BLOCKS 43200
setting INNBC_SNAPSHOT_BUILDER_SEGMENT_BLOCKS 1800

run_case 1 segmented
assert_not_invoked
grep -Fq 'INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB' "${TEST_ROOT}/stderr" ||
  fail "missing segmented allowance was not identified"

setting INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB ""
run_case 1 segmented
assert_not_invoked
grep -Fq 'INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB' "${TEST_ROOT}/stderr" ||
  fail "empty segmented allowance was not identified"
# FINISH ALPHA CHANGE 3079 -- reject missing or empty segmented allowances before dispatch --

grep -Fxq 'INNBC_SNAPSHOT_BUILDER_MDBX_MAP_SIZE_GB=1024' "${ENV_EXAMPLE}" ||
  fail "missing full-history builder allowance"
grep -Fxq 'INNBC_SNAPSHOT_BUILDER_SEGMENTED_MDBX_MAP_SIZE_GB=128' "${ENV_EXAMPLE}" ||
  fail "missing recent snapshot builder allowance"
# FINISH ALPHA CHANGE 3068 -- verify full-history and recent builder allowances independently --

echo "INNBC wrapper dispatch verification passed."
# FINISH ALPHA CHANGE 2090 -- exercise packaged dispatch using isolated temporary commands --