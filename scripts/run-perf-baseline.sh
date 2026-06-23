#!/usr/bin/env bash
# Phase 2: Run iperf3 and pktgen throughput/PPS baseline between two KVM guests.
# Run on the RHEL 9 host after both guests are up.
# Results are written to stdout and optionally to RESULTS_DIR.

set -euo pipefail

die() {
    printf "error: %s\n" "$*" >&2
    exit 1
}

info() {
    printf "[perf-baseline] %s\n" "$*"
}

# --- tunables ---
GUEST1_IP="${GUEST1_IP:-}"           # iperf3 server
GUEST2_IP="${GUEST2_IP:-}"           # iperf3 client
GUEST_SSH_USER="${GUEST_SSH_USER:-root}"
IPERF3_PORT="${IPERF3_PORT:-5201}"
IPERF3_PARALLEL="${IPERF3_PARALLEL:-4}"
IPERF3_DURATION="${IPERF3_DURATION:-30}"
RESULTS_DIR="${RESULTS_DIR:-./perf-results}"

[ -n "${GUEST1_IP}" ] || die "set GUEST1_IP to the iperf3 server guest IP"
[ -n "${GUEST2_IP}" ] || die "set GUEST2_IP to the iperf3 client guest IP"

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_cmd ssh
require_cmd date

mkdir -p "${RESULTS_DIR}"
TIMESTAMP=$(date '+%Y%m%d-%H%M%S')

ssh_guest() {
    local host="$1"; shift
    ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
        "${GUEST_SSH_USER}@${host}" "$@"
}

# --- Check iperf3 is present on guests ---
for guest in "${GUEST1_IP}" "${GUEST2_IP}"; do
    ssh_guest "${guest}" "command -v iperf3 >/dev/null 2>&1" \
        || die "iperf3 not found on ${guest}; install it before running this script"
done

# --- TCP throughput ---
info "starting iperf3 server on guest1 (${GUEST1_IP})"
ssh_guest "${GUEST1_IP}" \
    "pkill -f 'iperf3 -s' 2>/dev/null; iperf3 -s -D -p ${IPERF3_PORT}" \
    || die "failed to start iperf3 server"
sleep 2

info "running TCP throughput test: ${IPERF3_PARALLEL} parallel streams, ${IPERF3_DURATION}s"
TCP_RESULT="${RESULTS_DIR}/tcp-${TIMESTAMP}.json"
ssh_guest "${GUEST2_IP}" \
    "iperf3 -c ${GUEST1_IP} -p ${IPERF3_PORT} -P ${IPERF3_PARALLEL} -t ${IPERF3_DURATION} -J" \
    > "${TCP_RESULT}" \
    || die "iperf3 TCP test failed"

tcp_gbps=$(awk '/"bits_per_second"/ {s+=$2} END {printf "%.2f", s/1e9}' "${TCP_RESULT}" 2>/dev/null || echo "parse-error")
info "TCP throughput: ${tcp_gbps} Gbps"

# --- UDP PPS ---
info "running UDP PPS test (1-byte datagrams, ${IPERF3_DURATION}s)"
UDP_RESULT="${RESULTS_DIR}/udp-${TIMESTAMP}.json"
ssh_guest "${GUEST2_IP}" \
    "iperf3 -c ${GUEST1_IP} -p ${IPERF3_PORT} -u -b 0 -l 64 -t ${IPERF3_DURATION} -J" \
    > "${UDP_RESULT}" \
    || die "iperf3 UDP test failed"

udp_pps=$(awk '/"packets"/ {p=$2} END {printf "%d", p/'"${IPERF3_DURATION}"'}' "${UDP_RESULT}" 2>/dev/null || echo "parse-error")
info "UDP PPS: ~${udp_pps} pps"

# --- Cleanup ---
ssh_guest "${GUEST1_IP}" "pkill -f 'iperf3 -s' 2>/dev/null" || true

# --- Summary ---
echo ""
printf "=== perf baseline: %s ===\n" "${TIMESTAMP}"
printf "  TCP throughput : %s Gbps (%d parallel streams, %ds)\n" \
    "${tcp_gbps}" "${IPERF3_PARALLEL}" "${IPERF3_DURATION}"
printf "  UDP PPS        : %s pps (64-byte frames, %ds)\n" \
    "${udp_pps}" "${IPERF3_DURATION}"
printf "  raw results    : %s\n" "${RESULTS_DIR}/"
echo ""
info "compare these numbers against a software OVS baseline to confirm offload gain"
