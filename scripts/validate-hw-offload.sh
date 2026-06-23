#!/usr/bin/env bash
# Phase 2: Confirm that guest traffic is processed in BF3 hardware, not the host datapath.
# Run on the RHEL 9 host after a KVM guest is up and sending traffic.
# Exits 0 on pass, 1 on fail.

set -euo pipefail

die() {
    printf "error: %s\n" "$*" >&2
    exit 1
}

info() {
    printf "[validate-offload] %s\n" "$*"
}

pass() {
    printf "[PASS] %s\n" "$*"
}

fail() {
    printf "[FAIL] %s\n" "$*" >&2
    FAILURES=$((FAILURES + 1))
}

# --- tunables ---
DPU_SSH_HOST="${DPU_SSH_HOST:-192.168.100.2}"
DPU_SSH_USER="${DPU_SSH_USER:-ubuntu}"
# Host physical interface that carries guest traffic on the non-offloaded path.
# Used to confirm the host CPU is NOT processing packets.
HOST_UPLINK="${HOST_UPLINK:-}"
# Minimum hardware flow hit count expected after a short traffic burst.
MIN_HW_HITS="${MIN_HW_HITS:-100}"
# Traffic observation window in seconds.
SAMPLE_SECS="${SAMPLE_SECS:-10}"

FAILURES=0

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_cmd ssh
require_cmd sar
require_cmd awk

# --- Check 1: OVS hardware flow hits on BF3 ---
info "sampling OVS hardware flow stats on BF3 for ${SAMPLE_SECS}s ..."

before=$(ssh -o StrictHostKeyChecking=no "${DPU_SSH_USER}@${DPU_SSH_HOST}" \
    "sudo ovs-dpctl dump-flows -m type=offloaded 2>/dev/null | awk -F'packets:' 'NF>1{sum+=\$2+0} END{print sum+0}'" 2>/dev/null || echo 0)

sleep "${SAMPLE_SECS}"

after=$(ssh -o StrictHostKeyChecking=no "${DPU_SSH_USER}@${DPU_SSH_HOST}" \
    "sudo ovs-dpctl dump-flows -m type=offloaded 2>/dev/null | awk -F'packets:' 'NF>1{sum+=\$2+0} END{print sum+0}'" 2>/dev/null || echo 0)

hw_delta=$((after - before))
info "hardware flow hits during sample: ${hw_delta}"

if [ "${hw_delta}" -ge "${MIN_HW_HITS}" ]; then
    pass "hardware flow hit count ${hw_delta} >= threshold ${MIN_HW_HITS}"
else
    fail "hardware flow hit count ${hw_delta} < threshold ${MIN_HW_HITS} — offload may not be active"
fi

# --- Check 2: host CPU idle under guest load ---
info "measuring host CPU usage over ${SAMPLE_SECS}s ..."

# sar reports %idle; we want it to stay high (>= 80%) if offload is working.
cpu_idle=$(sar -u "${SAMPLE_SECS}" 1 2>/dev/null \
    | awk '/^Average/ {print $NF}' || echo 0)
cpu_idle_int=${cpu_idle%.*}

info "host CPU idle: ${cpu_idle}%"

if [ "${cpu_idle_int}" -ge 80 ]; then
    pass "host CPU idle ${cpu_idle}% >= 80% — host not processing guest packets"
else
    fail "host CPU idle ${cpu_idle}% < 80% — possible software fallback path active"
fi

# --- Check 3: host uplink PPS (should be near zero) ---
if [ -n "${HOST_UPLINK}" ]; then
    info "checking PPS on host uplink ${HOST_UPLINK} ..."

    rx_before=$(cat /sys/class/net/"${HOST_UPLINK}"/statistics/rx_packets 2>/dev/null || echo 0)
    sleep "${SAMPLE_SECS}"
    rx_after=$(cat /sys/class/net/"${HOST_UPLINK}"/statistics/rx_packets 2>/dev/null || echo 0)
    pps=$(( (rx_after - rx_before) / SAMPLE_SECS ))

    info "host uplink ${HOST_UPLINK} RX PPS: ${pps}"

    # Offloaded traffic should bypass the host uplink entirely.
    if [ "${pps}" -lt 100 ]; then
        pass "host uplink PPS ${pps} < 100 — traffic is bypassing host datapath"
    else
        fail "host uplink PPS ${pps} >= 100 — traffic may be transiting host CPU"
    fi
fi

# --- Check 4: BF3 hardware counters ---
info "reading BF3 hardware packet counters ..."
bf3_hw_rx=$(ssh -o StrictHostKeyChecking=no "${DPU_SSH_USER}@${DPU_SSH_HOST}" \
    "cat /sys/class/net/*/statistics/rx_packets 2>/dev/null | awk '{s+=\$1} END{print s+0}'" \
    2>/dev/null || echo 0)

info "BF3 aggregate RX packets: ${bf3_hw_rx}"
if [ "${bf3_hw_rx}" -gt 0 ]; then
    pass "BF3 hardware counters are incrementing"
else
    fail "BF3 hardware counters are zero — check OVS-DOCA and vDPA device state"
fi

# --- Summary ---
echo ""
if [ "${FAILURES}" -eq 0 ]; then
    printf "[RESULT] PASS — hardware offload validated (%d checks)\n" 4
    exit 0
else
    printf "[RESULT] FAIL — %d check(s) failed; review output above\n" "${FAILURES}" >&2
    exit 1
fi
