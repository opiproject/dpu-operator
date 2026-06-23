#!/usr/bin/env bash
# Phase 1: Provision BF3 in DPU mode and deploy OVS-DOCA on the ARM cores.
# Run on the RHEL 9 host with BF3 installed and RShim driver loaded.

set -euo pipefail

die() {
    printf "error: %s\n" "$*" >&2
    exit 1
}

info() {
    printf "[setup-bf3] %s\n" "$*"
}

# --- tunables ---
BF3_PCIE_ADDR="${BF3_PCIE_ADDR:-}"          # e.g. 0000:03:00.0
NUM_VFS="${NUM_VFS:-4}"
RSHIM_DEV="${RSHIM_DEV:-/dev/rshim0/boot}"
DPU_SSH_HOST="${DPU_SSH_HOST:-192.168.100.2}"   # OOB management IP on BF3 ARM
DPU_SSH_USER="${DPU_SSH_USER:-ubuntu}"
OVS_DOCA_SERVICE="${OVS_DOCA_SERVICE:-openvswitch}"

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_cmd mlxconfig
require_cmd mlxfwreset
require_cmd ssh
require_cmd lspci

# --- Step 1: locate BF3 PCIe address ---
if [ -z "${BF3_PCIE_ADDR}" ]; then
    BF3_PCIE_ADDR=$(lspci -D | awk '/BlueField.*ConnectX/ || /MT424[0-9]/ {print $1; exit}')
    [ -n "${BF3_PCIE_ADDR}" ] || die "could not detect BF3 PCIe address; set BF3_PCIE_ADDR"
    info "detected BF3 at ${BF3_PCIE_ADDR}"
fi

# --- Step 2: set DPU mode via mlxconfig ---
info "setting INTERNAL_CPU_MODEL=1 (DPU mode) on ${BF3_PCIE_ADDR}"
mlxconfig -d "${BF3_PCIE_ADDR}" -y set INTERNAL_CPU_MODEL=1 \
    INTERNAL_CPU_OFFLOAD_ENGINE=0 \
    || die "mlxconfig failed; check PCIe address and privileges"

# --- Step 3: configure SR-IOV ---
info "enabling SR-IOV with ${NUM_VFS} VFs on ${BF3_PCIE_ADDR}"
mlxconfig -d "${BF3_PCIE_ADDR}" -y set SRIOV_EN=1 NUM_OF_VFS="${NUM_VFS}" \
    || die "SR-IOV mlxconfig failed"

# --- Step 4: reset firmware to apply changes ---
info "resetting firmware (mlxfwreset); the link will drop briefly"
mlxfwreset -d "${BF3_PCIE_ADDR}" -y reset || die "mlxfwreset failed"

# --- Step 5: wait for BF3 ARM to come back up ---
info "waiting for BF3 ARM SSH at ${DPU_SSH_HOST} ..."
RETRIES=30
for i in $(seq 1 ${RETRIES}); do
    if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 \
           "${DPU_SSH_USER}@${DPU_SSH_HOST}" true 2>/dev/null; then
        break
    fi
    [ "${i}" -lt "${RETRIES}" ] || die "BF3 ARM did not come up after ${RETRIES} attempts"
    sleep 10
done
info "BF3 ARM is reachable"

# --- Step 6: set switchdev eswitch mode on the host PF ---
HOST_PF=$(ls /sys/bus/pci/devices/"${BF3_PCIE_ADDR}"/net/ 2>/dev/null | head -1)
[ -n "${HOST_PF}" ] || die "host PF netdev not found under ${BF3_PCIE_ADDR}"
info "switching ${HOST_PF} to switchdev mode"
devlink dev eswitch set "pci/${BF3_PCIE_ADDR}" mode switchdev \
    || die "devlink eswitch set failed"

# --- Step 7: deploy and start OVS-DOCA on the BF3 ARM ---
info "starting ${OVS_DOCA_SERVICE} on BF3 ARM"
ssh -o StrictHostKeyChecking=no "${DPU_SSH_USER}@${DPU_SSH_HOST}" \
    "sudo systemctl enable --now ${OVS_DOCA_SERVICE}" \
    || die "failed to start OVS-DOCA on BF3 ARM"

# --- Step 8: create the uplink bridge on BF3 ---
info "creating OVS bridge br-dpu on BF3 ARM"
ssh -o StrictHostKeyChecking=no "${DPU_SSH_USER}@${DPU_SSH_HOST}" "
    set -e
    sudo ovs-vsctl --may-exist add-br br-dpu
    PF=\$(ls /sys/bus/pci/devices/*/net/ 2>/dev/null | grep -v representor | head -1 || true)
    [ -n \"\${PF}\" ] && sudo ovs-vsctl --may-exist add-port br-dpu \"\${PF}\"
    sudo ovs-vsctl set Open_vSwitch . other_config:hw-offload=true
    sudo systemctl restart ${OVS_DOCA_SERVICE}
" || die "OVS bridge setup on BF3 ARM failed"

# --- Step 9: verify ---
info "verifying OVS bridge on BF3 ARM"
ssh -o StrictHostKeyChecking=no "${DPU_SSH_USER}@${DPU_SSH_HOST}" \
    "sudo ovs-vsctl show" \
    || die "ovs-vsctl show failed; OVS-DOCA may not be healthy"

info "Phase 1 complete: BF3 in DPU mode, OVS-DOCA running, br-dpu configured"
info "  Next: run scripts/validate-hw-offload.sh after attaching a KVM guest"
