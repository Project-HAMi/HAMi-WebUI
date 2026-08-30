<img src="docs/logo-horizontal.png" alt="HAMi-WebUI Logo (Light)" width="50%">

[English](README.md) | 简体中文 | [日本語](README_JA.md)

面向 [HAMi](https://github.com/Project-HAMi/HAMi) 管理的异构加速器的开源单集群可观测界面

[![License](https://img.shields.io/github/license/Project-HAMi/HAMi-WebUI)](LICENSE)

HAMi-WebUI 用于查看 Kubernetes 节点上的加速器清单、分配状态，以及各厂商当前可提供的监控数据。主要包括：

- **集群概览：** 查看已识别加速器的容量、分配情况和可获取的使用率指标。
- **节点清单：** 查看加速器节点、设备及其资源状态。
- **加速器可见性：** 查看每个设备的分配情况和厂商监控数据。
- **工作负载可见性：** 查看当前 HAMi Pod/容器的设备分配，以及对应设备可提供的监控数据。

当前适配 NVIDIA GPU、华为昇腾 910B/310P、海光 DCU、寒武纪 MLU，以及沐曦 GPU/sGPU。不同设备类型可展示的指标取决于对应的 HAMi 适配和 exporter。

## 范围与安全边界

HAMi-WebUI 是只读界面，不负责调度资源、创建或修改工作负载、变更节点，也不内置登录、用户 RBAC 或多集群聚合。每个集群应独立部署一个实例；除非仅在可信网络内访问，否则请在前方配置带身份认证的 Ingress 或代理。

## 开始使用

- [安装指南](docs/installation/helm/index.md)
- [URL 前缀与平台嵌入](docs/installation/embedding.md)

## 参与贡献

如果你对 HAMi-WebUI 项目感兴趣：

- 首先阅读[贡献指南](CONTRIBUTING.md)。
- 按照我们的[开发者指南](docs/contribute/developer-guide.md)设置本地开发环境。
- 查看[适合首次贡献的问题](https://github.com/Project-HAMi/HAMi-WebUI/issues?q=is%3Aopen+is%3Aissue+label%3A%22good+first+issue%22)。

## 参与社区

HAMi-WebUI 使用 HAMi 社区的统一沟通渠道。最新的社区会议、Discord、Slack 和邮件列表入口请查看 [HAMi 社区说明](https://github.com/Project-HAMi/HAMi/blob/master/README_cn.md#社区)。

## 许可证

HAMi-WebUI 根据 [Apache-2.0](LICENSE) 许可证分发。如需了解详细情况，请参阅 [LICENSE](LICENSE)。
