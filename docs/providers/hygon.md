# Hygon DCU and HCU compatibility

HAMi-WebUI keeps legacy DCU and current HCU integrations separate. Configure the
selector that matches your node labels:

```yaml
vendorNodeSelectors:
  DCU: dcu=on
  HCU: hcu=on
```

Direct backend configuration uses the same keys under `node_selectors`. An older
config file without `HCU` uses `hcu=on` for that provider. HCU devices
are discovered from `hami.io/node-hcu-register`, which the Hygon device plugin
writes in `strategy=hami`. Physical-only, pre-split and MIG plugin modes without
HAMi registration/allocation annotations are outside this integration.
The default `hcu=on` selector matches the vendor's
[HAMi-mode deployment](https://github.com/HYGON-AI/k8s-hcu-device-plugin/blob/cc7d33f4b00acca00b8657144aed72e4e2d5604f/deployment/static/k8s-hcu-plugin-hami.yaml).
Override it when your cluster uses different labels.

The browser receives the actual provider (`DCU` or `HCU`) and registered model,
such as `HCU-K100_AI`. Both remain available in the existing inventory and model
filters; the frontend has no vendor allowlist requiring separate HCU UI code.

## Registration and allocation identities

| Contract | Legacy DCU | HCU |
| --- | --- | --- |
| Node registration | `hami.io/node-dcu-register` | `hami.io/node-hcu-register` |
| Pod allocation | `hami.io/dcu-devices-allocated` | `hami.io/hcu-devices-allocated` |
| Allocation identity | Node-local `DCU-<index>` mapped to `<node>-dcu-<index>` | Stable `HCU-<serial>` |
| Physical exporter identity | `dcu_temp{node,minor_number}` maps to `device_id` | Strip only the leading `HCU-` to obtain exporter `device_id`; preserve the full ID for allocation joins |

HCU registration includes device index, split count, schedulable memory in MiB,
core percentage, model, NUMA and health. It remains visible when telemetry is
absent. Pod decoding preserves init-container slots and whole-card allocations.

## HCU exporter contract

Use the default metric and label names from `HYGON-AI/hcu-exporter`. Its optional
`--metrics-define` and `--label-define` renaming is not inferred by WebUI. The
exporter prefixes its workload namespace and Pod labels, but its `container`
label collides with the target label that Prometheus Operator attaches. WebUI
accepts the workload container as `container` (scraped with `honor_labels: true`)
or as `exported_container` (scraped without it).

| Metric | Required identity | Value consumed by WebUI |
| --- | --- | --- |
| `hcu_memorycap_bytes`, `hcu_usedmemory_bytes` | `device_id` | Bytes, converted to MiB |
| `hcu_utilizationrate` | `device_id` | Physical-device utilization percent |
| `hcu_temp`, `hcu_temp_mem` | `device_id` | Degrees Celsius |
| `hcu_power_usage` | `device_id`, `minor_number` | Watts; minor number supplies the displayed device number |
| `vhcu_utilizationrate` | `device_id`, `node`, `hcu_pod_namespace`, `hcu_pod_name`, `container` | Busy percent of the vHCU's own compute units, averaged over the last minute. WebUI uses it as allocated-compute activity and estimates active vCore as busy percent × allocated vCore / 100 |
| `vhcu_usedmemory_bytes` | Same workload identity | Bytes, converted to MiB |

`vhcu_usedmemory_percent` is not consumed. The exporter reports it as a 0-1 ratio
despite its name, and WebUI already derives memory utilization from bytes and the
allocated MiB.

### Allocations with workload telemetry

The device plugin decides whether a container receives a vHCU, a whole-card mark
or neither. The exporter attaches workload labels only in the first two cases:

| HAMi allocation | Plugin action | Workload series |
| --- | --- | --- |
| One device with less than the card's memory | Creates a vHCU with `max(1, ceil(cores × compute units / 100))` compute units | `vhcu_*` |
| `hcucores` 100 and the card's full memory, per device | Records a whole-card mark | `hcu_*` with workload labels |
| Any other shape, such as several partial devices, or fewer than 100 cores with full memory | Neither | None; usage stays unavailable |

WebUI prefers the vHCU sample, then the labelled whole-card sample. Both must
match the physical device, node, namespace, Pod and container. The exporter reads
an instantaneous busy percent, so container utilization averages each sample
series over the last minute, as NVIDIA container utilization does; memory uses the
latest sample. Unattributed whole-card telemetry is never used as container usage.
A missing workload sample stays unavailable; a reported zero is idle. The exporter
resets every `hcu_*` and `vhcu_*` series on each collection loop, so labels of
finished Pods do not linger.

### vHCU utilization semantics

`vhcu_utilizationrate` is read from `dmiGetVDevBusyPercent` in the closed-source
`libhydmi`. The published sources describe it relative to the vHCU, not to the
physical card:

- The DMI header documents the virtual and physical calls identically, as the
  queried device's busy percent from 0 to 100.
- The exporter README's sample reports 72 for a vHCU with 8 compute units. That
  value cannot be a share of a physical card with dozens of compute units.
- The vendor Grafana dashboards chart the raw value directly as a 0-100 percent.

WebUI therefore handles it like NVIDIA container utilization: utilization is the
busy percent without rescaling, and used vCore scales it by the allocation. This
is not yet confirmed on hardware. A single-device allocation with
`hygon.com/hcucores: 25` and partial memory that saturates its vHCU should read
about 100, not 25.

The plugin rounds compute units up, so a vHCU can exceed its allocated percentage
by less than one compute unit. An explicit `hygon.com/hcucores: 0` with partial
memory yields a one-compute-unit vHCU. WebUI follows the HAMi allocation encoding
and reports that allocation as a whole card, which overstates its used vCore.

The exporter does not expose a Pod UID label. Workload identity therefore uses
its node/namespace/Pod/container labels, plus the physical device serial. Driver
version, fan and XID fields are not synthesized from unrelated metrics.

## Pinned source evidence

These source revisions define the compatibility fixtures; they are not a claim
of physical-hardware acceptance:

- [HAMi HCU adapter, `88b118e`](https://github.com/Project-HAMi/HAMi/blob/88b118e565a9effaa27f81669da291c5508fc5e4/pkg/device/hygon/device.go): registration, resource and allocation names.
- [Hygon device registration, `cc7d33f`](https://github.com/HYGON-AI/k8s-hcu-device-plugin/blob/cc7d33f4b00acca00b8657144aed72e4e2d5604f/internal/pkg/plugin/register.go): serial-prefixed ID, model, capacity and health; [wire encoding](https://github.com/HYGON-AI/k8s-hcu-device-plugin/blob/cc7d33f4b00acca00b8657144aed72e4e2d5604f/internal/pkg/util/util.go).
- [HCU exporter labels, `bfe780b`](https://github.com/HYGON-AI/hcu-exporter/blob/bfe780b89831a6245f19498c1068d17836d84653/cmd/hcu-exporter/metrics_loop.go): physical and virtual device/workload identities; [metric readers](https://github.com/HYGON-AI/hcu-exporter/blob/bfe780b89831a6245f19498c1068d17836d84653/cmd/hcu-exporter/main.go).
- [hcu-dcgm v3.0.0, `b223b72`](https://github.com/HYGON-AI/hcu-dcgm/blob/b223b72ee0c0079ad1b93a637fc96e00636f7a65/pkg/dcgm/api.go): serial-number identity, watt conversion and virtual-device busy percent.
- [Hygon device plugin allocation, `cc7d33f`](https://github.com/HYGON-AI/k8s-hcu-device-plugin/blob/cc7d33f4b00acca00b8657144aed72e4e2d5604f/internal/pkg/plugin/plugin.go#L755-L773): whole-card marks and vHCU compute-unit sizing.
- [DMI virtual-device header, `b223b72`](https://github.com/HYGON-AI/hcu-dcgm/blob/b223b72ee0c0079ad1b93a637fc96e00636f7a65/pkg/dcgm/include/dmi_virtual.h#L223-L246), [exporter vHCU sample](https://github.com/HYGON-AI/hcu-exporter/blob/bfe780b89831a6245f19498c1068d17836d84653/README.md?plain=1#L546) and [vendor dashboard](https://github.com/HYGON-AI/hcu-exporter/blob/bfe780b89831a6245f19498c1068d17836d84653/grafana/hcu-exporter-k8s-dashboard.json): vHCU-relative busy percent.
