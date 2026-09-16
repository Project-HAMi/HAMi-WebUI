# Huawei Ascend NPU

HAMi-WebUI reads Ascend models from HAMi's device configuration, the same file
HAMi's scheduler and the ascend-device-plugin use. It has no built-in model
list: a model appears as soon as it is in that configuration, and the compute
share of a template comes from the template and model AI Core counts there.

## Device configuration

HAMi renders the configuration as ConfigMap `<HAMi release>-scheduler-device`
with the key `device-config.yaml`. The Chart defaults match a HAMi release named
`hami` in `kube-system`:

```yaml
hami:
  deviceConfig:
    enabled: true
    namespace: kube-system
    name: hami-scheduler-device
    key: device-config.yaml
```

The Chart grants the WebUI service account `get`, `list` and `watch` on that one
ConfigMap through a Role in HAMi's namespace, so installing the Chart requires
permission to create Roles there. WebUI watches the ConfigMap and applies edits
without a restart; HAMi's scheduler reads it only at startup. Setting
`enabled: false` removes the Role, and template compute shares then show as
unknown. `GET /api/vgpu/v1/device-config` returns what WebUI read: its state,
the Ascend models and templates, and any problems found in the file.

| State | Meaning |
| --- | --- |
| `loaded` | Read and parsed. A later transient API error keeps the last read. |
| `missing` | The ConfigMap does not exist. Check `namespace` and `name`. |
| `forbidden` | The service account cannot read it. Check the Role and RoleBinding. |
| `invalid` | The key is missing or its YAML cannot be parsed. |
| `loading` | Not read yet. |
| `error` | The first read failed. WebUI keeps retrying. |
| `disabled` | `hami.deviceConfig.enabled` is false. |

While the configuration cannot be read, the accelerator list and the overview
carry a note saying why, because compute shares of template splits then read
`--` and allocation rates understate.

## Devices

Nodes register NPUs in `hami.io/node-register-<commonWord>`. WebUI reads every
`commonWord` in the configuration, plus unlisted ones starting with `Ascend` so
that leftover registrations stay visible. When the configuration is loaded, a
registered model it does not list is marked **Not configured** and adds no
allocatable capacity, because HAMi's scheduler does not allocate it either.

## Allocations

Pod allocations come from `hami.io/<commonWord>-devices-allocated` (one segment
per container, init containers first) and the applied template names in
`huawei.com/<commonWord>`.

| Shape | How it is recognised | Compute share |
| --- | --- | --- |
| Soft split | `huawei.com/vnpu-mode: hami-core`, or a `hami-vnpu-core` node as below | The requested share. 0 reserves none and shows as not limited: the runtime then shares the NPU by default priority |
| Whole card | Otherwise, HAMi's entry for the device in `huawei.com/<commonWord>` names no template. Without that entry, memory equals the model's `memoryAllocatable` or the card's registered memory | 100% |
| Template | Otherwise, by the template name in that entry, or a template of exactly that memory | Template `aiCore` / model `aiCore` |

A malformed allocation annotation hides only that model's devices; the Pod's
other devices stay visible.

Pages about Ascend devices call them NPUs. When `scheduling.resourceNames`
includes a model's resource names, a pending Pod's request shows as NPU count,
per-card memory and per-card compute instead of raw resource names.

Workload rows report the shape, the template and, when the compute share is
unknown, why:

| Reason | Meaning |
| --- | --- |
| `catalog_unavailable` | The device configuration is not loaded. |
| `model_not_configured` | The configuration does not list the model. |
| `template_not_configured` | No configured template matches the allocation. |
| `compute_not_configured` | The model or template has no `aiCore`. |
| `mode_ambiguous` | See below. |
| `node_mode_unknown` | The node could not be read, or it has no `hami-vnpu-core` annotation and the configuration is not loaded. |

### Pods without `huawei.com/vnpu-mode`

A node with `hami-vnpu-core: "true"` has soft split enabled. There, a Pod that
does not set `huawei.com/vnpu-mode` gets a template split from ascend-device-plugin
v1.4.0 but a soft split from v1.4.1, and both leave the same annotations
([ascend-device-plugin#134](https://github.com/Project-HAMi/ascend-device-plugin/issues/134)).
WebUI reports these allocations as `mode_ambiguous` unless you set:

```yaml
ascend:
  annotationlessPodMode: node      # ascend-device-plugin v1.4.1 and later
  # annotationlessPodMode: template  # v1.4.0
```

HAMi installs with `hamiVnpuCore: false`, where this never applies. A node
without the annotation follows the configured `hamiVnpuCore`, as HAMi's scheduler
does. The node's annotation is read when the allocation is decoded, so after a
node switches modes its existing allocations keep the earlier reading until that
Pod changes or the hourly resync decodes it again.

## Upgrading from a built-in model table

Earlier releases compiled in the templates of seven models. They match the
configuration HAMi v2.10 ships, so a default HAMi installation shows the same
values. If HAMi uses another release name or namespace, set `hami.deviceConfig`;
until WebUI can read the configuration, template compute shares show `--` with
`catalog_unavailable` instead of the built-in values.
