#!/usr/bin/env bash
# Phase 2: Live-migrate a KVM guest with vDPA and measure connectivity loss.
# Run on the RHEL 9 source host. Requires shared storage between source and dest.
# Exits 0 if migration completes and packet loss is within LOSS_THRESHOLD.

set -euo pipefail

die() {
    printf "error: %s\n" "$*" >&2
    exit 1
}

info() {
    printf "[live-migration] %s\n" "$*"
}

# --- tunables ---
VM_NAME="${VM_NAME:-}"                         # libvirt domain name
DEST_HOST="${DEST_HOST:-}"                     # URI: qemu+ssh://root@<host>/system
GUEST_IP="${GUEST_IP:-}"                       # guest IP to ping during migration
PING_INTERVAL="${PING_INTERVAL:-0.2}"          # seconds between pings
MIGRATION_TIMEOUT="${MIGRATION_TIMEOUT:-300}"  # seconds before declaring failure
# Maximum allowed packet loss percentage across the migration window.
LOSS_THRESHOLD="${LOSS_THRESHOLD:-5}"

[ -n "${VM_NAME}" ]   || die "set VM_NAME to the libvirt domain name"
[ -n "${DEST_HOST}" ] || die "set DEST_HOST to the destination libvirt URI (e.g. qemu+ssh://root@host2/system)"
[ -n "${GUEST_IP}" ]  || die "set GUEST_IP to the guest's IP address"

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_cmd virsh
require_cmd ping
require_cmd awk

# --- Confirm guest is running ---
virsh domstate "${VM_NAME}" | grep -q running \
    || die "domain ${VM_NAME} is not running"

# --- Start background ping to measure loss during migration ---
PING_LOG=$(mktemp /tmp/live-migration-ping.XXXX)
info "pinging ${GUEST_IP} every ${PING_INTERVAL}s (log: ${PING_LOG})"
ping -i "${PING_INTERVAL}" "${GUEST_IP}" > "${PING_LOG}" 2>&1 &
PING_PID=$!

cleanup() {
    kill "${PING_PID}" 2>/dev/null || true
    wait "${PING_PID}" 2>/dev/null || true
}
trap cleanup EXIT

sleep 2
kill -0 "${PING_PID}" 2>/dev/null || die "ping to ${GUEST_IP} failed immediately; check guest connectivity"
info "baseline connectivity confirmed"

# --- Initiate live migration ---
info "migrating ${VM_NAME} to ${DEST_HOST} (timeout: ${MIGRATION_TIMEOUT}s)"
MIGRATE_START=$(date +%s)

virsh migrate --live --persistent --undefinesource \
    --timeout "${MIGRATION_TIMEOUT}" \
    "${VM_NAME}" "${DEST_HOST}" \
    || die "virsh migrate failed; check shared storage and destination host connectivity"

MIGRATE_END=$(date +%s)
MIGRATE_DURATION=$(( MIGRATE_END - MIGRATE_START ))
info "migration completed in ${MIGRATE_DURATION}s"

# --- Stop ping and parse results ---
kill "${PING_PID}" 2>/dev/null || true
wait "${PING_PID}" 2>/dev/null || true

sent=$(awk '/packets transmitted/ {print $1}' "${PING_LOG}" || echo 0)
received=$(awk '/packets transmitted/ {print $4}' "${PING_LOG}" || echo 0)
loss_pct=$(awk '/packets transmitted/ {print $6+0}' "${PING_LOG}" || echo 100)

info "ping stats: ${sent} sent, ${received} received, ${loss_pct}% loss"

# --- Verify guest is accessible on destination ---
info "verifying guest is reachable post-migration ..."
for i in $(seq 1 10); do
    ping -c 1 -W 2 "${GUEST_IP}" >/dev/null 2>&1 && break
    [ "${i}" -lt 10 ] || die "guest ${GUEST_IP} unreachable after migration"
    sleep 3
done
info "guest is reachable on destination host"

# --- Evaluate result ---
loss_int=${loss_pct%.*}
echo ""
printf "=== live migration result ===\n"
printf "  domain         : %s\n" "${VM_NAME}"
printf "  destination    : %s\n" "${DEST_HOST}"
printf "  duration       : %ds\n" "${MIGRATE_DURATION}"
printf "  packet loss    : %s%% (threshold: %s%%)\n" "${loss_pct}" "${LOSS_THRESHOLD}"

if [ "${loss_int}" -le "${LOSS_THRESHOLD}" ]; then
    printf "  result         : PASS\n"
    exit 0
else
    printf "  result         : FAIL — packet loss %s%% exceeds threshold %s%%\n" \
        "${loss_pct}" "${LOSS_THRESHOLD}" >&2
    exit 1
fi
