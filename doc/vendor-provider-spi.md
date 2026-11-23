# VendorProvider SPI (v0)

Minimal handshake so `dpu-operator` can consume an **out-of-tree** vendor
companion (NVIDIA DPF adapter) without merging that codebase into this
repository.

## Goals

- User applies only OPI CRs (`DpuOperatorConfig`, `DataProcessingUnit`, …).
- Intel / Marvell keep today’s in-tree VSP/daemon path (**no** `VendorProvider`).
- NVIDIA: OPI installs the companion Deployment by convention; companion
  registers `VendorProvider/nvidia`; core gates on `Available`.

## API

### `DpuOperatorConfig.spec.vendor`

| Value | Behavior |
|---|---|
| empty / `intel` / `marvell` | Path A — never looks up `VendorProvider`, never deploys companion |
| `nvidia` | Require `VendorProvider/nvidia` Available. Ensure an in-namespace companion Deployment only if the provider is not yet Available (skip when an out-of-tree companion in another namespace already registered it). |

### `VendorProvider` (cluster-scoped)

Companion creates/updates this object:

```yaml
apiVersion: config.openshift.io/v1
kind: VendorProvider
metadata:
  name: nvidia
spec:
  vendor: nvidia
  capabilities: [lifecycle, translate]
  endpoint: ""   # empty = watch same OPI CRs
status:
  conditions:
  - type: Available
    status: "True"
```

Core sets `DpuOperatorConfig` condition `VendorProviderAvailable` only when
`spec.vendor` requires a provider. `Ready` stays owned by the existing
reconcile path; if the provider is required and missing/not Available,
`Ready` becomes False with reason `VendorProviderError`.

## Detection and the vendor plugin

`spec.detection` is optional. When it is set and the provider is `Available`,
the daemon adds a detector for that product. Intel, Marvell, and NetSec stay
on the built-in detectors, and a provider that omits `detection` changes nothing.

```yaml
spec:
  vendor: nvidia
  capabilities: [lifecycle, translate]
  detection:
    pciVendorID: "15b3"
    pciDeviceIDs: ["a2dc", "a2dd"]
    excludePCIDeviceIDs: ["c2d3"]
    productName: NVIDIA BlueField-3
    platformName: nvidia-bluefield
```

| Field | Meaning |
|---|---|
| `pciVendorID`, `pciDeviceIDs` | Host PCI IDs, four hex digits. Matched case-insensitively. |
| `excludePCIDeviceIDs` | Same vendor, but not a DPU. A management function goes here. |
| `productName` | Written to `DataProcessingUnit.spec.dpuProductName`. |
| `platformName` | DNS label. The CR name is `<platformName>-<lowercase serial>-host`. |

Detection is host side only. The daemon never creates a DPU-side
`DataProcessingUnit` from a provider. Two ports of one card share a serial, so
they become one CR. SR-IOV virtual functions are ignored even when their device
ID is listed. A device whose serial cannot be read is skipped.

The daemon reads the serial from the sysfs `serial` file, then VPD `SN`, then
the PCIe Device Serial Number capability. It does not use the fixed config
offset the Intel path uses.

A provider with `detection` set owns the vendor plugin for `productName`. The
`DataProcessingUnit` controller does not render a VSP pod for that product.
The provider deploys its own plugin on the socket the daemon already dials.
Intel and Marvell keep the in-tree VSP pod.

The daemon owns only the `Ready` condition on a `DataProcessingUnit`. Each
status sync sets or removes that condition and leaves every other condition as
it is, so a vendor adapter keeps the conditions it writes. When `Ready` is
already unchanged, the daemon does not write status.

The daemon needs `get`, `list`, and `watch` on `vendorproviders`. That rule is
in the daemon ClusterRole. If the list fails, the daemon keeps the built-in
detectors and logs the error once.

## What stays out of core

- DPF Helm charts, Kamaji, Multus policy, FieldMapping
- Changes under `internal/daemon/vendor-specific-plugins/`

## Sample

See [`examples/config-nvidia.yaml`](../examples/config-nvidia.yaml).

Companion image env key: `NvidiaDpfAdapterImage` (same image-manager pattern
as other operand images). Default in `config/manager/manager.yaml`:
`quay.io/opiproject/opi-nvidia-dpf-adapter:latest` (override for lab pins).

## Phase 2 — in-namespace ensure

When `spec.vendor=nvidia` and `VendorProvider/nvidia` is **not** yet
`Available`, the config controller applies bindata under
`internal/controller/bindata/nvidia-adapter/` (SA, ClusterRole, binding,
Deployment) into the operator namespace.

When an out-of-tree companion (e.g. `opi-nvidia-dpf-adapter-system`) has
already registered `VendorProvider/nvidia` Available, ensure **skips** so
two managers do not race.

**Image contract:** the companion image must include FieldMapping YAML at
`/mappings` (see companion `Dockerfile`). Bindata passes
`--mapping-dir=/mappings`. ClusterRole mirrors the companion manager +
Helm-install surface so LCM can install the pinned DPF chart.

## Lab notes

1. Prefer one companion: either out-of-tree deploy **or** in-namespace
   ensure — not both.
2. Point `NvidiaDpfAdapterImage` at a build that includes `/mappings`.
3. After companion is Up, expect `DpuOperatorConfig` condition
   `VendorProviderAvailable=True`.
