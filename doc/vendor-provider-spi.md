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
