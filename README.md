<img src="docs/logo-horizontal.png" alt="HAMi-WebUI Logo (Light)" width="50%">

English | [简体中文](README_ZH.md) | [日本語](README_JA.md)

An open-source, single-cluster observability UI for heterogeneous accelerators managed by [HAMi](https://github.com/Project-HAMi/HAMi)

[![License](https://img.shields.io/github/license/Project-HAMi/HAMi-WebUI)](LICENSE)

HAMi-WebUI visualizes accelerator inventory, allocation state, and available
vendor telemetry across Kubernetes nodes. It provides:

- **Cluster overview:** Review detected accelerator capacity, allocation, and
  available utilization signals.
- **Node inventory:** Inspect accelerator nodes, their devices, and resource
  state.
- **Accelerator visibility:** Inspect per-device allocation and vendor telemetry.
- **Workload visibility:** Inspect current HAMi Pod/container assignments and the
  monitoring data available for their devices.

Current provider integrations include NVIDIA GPUs, Huawei Ascend 910B/310P,
Hygon DCUs, Cambricon MLUs, and MetaX GPUs/sGPUs. Metric coverage depends on the
HAMi integration and exporter available for each device type.

## Scope and security

HAMi-WebUI is read-only. It does not schedule resources, create or mutate
workloads, change nodes, provide built-in authentication or user RBAC, or
aggregate multiple clusters. Deploy one instance per cluster and protect it
with an authenticated Ingress or proxy unless it is reachable only from a
trusted network.

## Get started

- [Installation guide](docs/installation/helm/index.md)
- [URL prefix and platform embedding](docs/installation/embedding.md)

## Contributing

If you're interested in contributing to HAMi-WebUI:

- Start by reading the [Contributing guide](CONTRIBUTING.md).
- Set up your local environment by following our [Developer guide](docs/contribute/developer-guide.md).
- Explore [good first issues](https://github.com/Project-HAMi/HAMi-WebUI/issues?q=is%3Aopen+is%3Aissue+label%3A%22good+first+issue%22).

## Get involved

HAMi-WebUI is discussed in the HAMi community. See the canonical
[HAMi community channels](https://github.com/Project-HAMi/HAMi#community) for
current meetings, Discord, Slack, and the mailing list.

## License

HAMi-WebUI is distributed under [Apache-2.0](LICENSE). For details, see [LICENSE](LICENSE).
