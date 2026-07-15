# HAMi-WebUI

![Version: 1.0.4](https://img.shields.io/badge/Version-1.0.4-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.4](https://img.shields.io/badge/AppVersion-1.0.4-informational?style=flat-square)

## Get Repo Info

```console
helm repo add hami-webui https://project-hami.github.io/HAMi-WebUI
helm repo update
```

_See [helm repo](https://helm.sh/docs/helm/helm_repo/) for command documentation._

## Installing the Chart

Before deploying, ensure that you configure the `values.yaml` file to match your cluster’s requirements. For detailed instructions, refer to the [Configuration Guide for HAMi-WebUI Helm Chart](#configuration-guide-for-hamiwebui-helm-chart)
> _**Important**_: You must adjust the values.yaml before proceeding with the deployment.

Download the `values.yaml` file from the Helm Charts repository:

https://github.com/Project-HAMi/HAMi-WebUI/blob/main/charts/hami-webui/values.yaml


```console
helm install my-hami-webui hami-webui/hami-webui --create-namespace --namespace hami -f values.yaml
```

## Uninstalling the Chart

To uninstall/delete the my-release deployment:

```console
helm delete my-hami-webui
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://nvidia.github.io/dcgm-exporter/helm-charts | dcgm-exporter | 3.5.0 |
| https://prometheus-community.github.io/helm-charts | kube-prometheus-stack | 62.6.0 |

## Values

| Key | Type | Default                                                                            | Description |
|-----|------|------------------------------------------------------------------------------------|-------------|
| affinity | object | `{}`                                                                               |  |
| basePath | string | `"/"`                                                                              | URL sub-path to serve the WebUI under, injected as `HAMI_WEBUI_BASE_PATH`. `"/"` = root (default). Set e.g. `"/gpu-ui/"` for a non-stripping reverse proxy. Resolved at request time (no image rebuild). A path-stripping proxy that sets `X-Forwarded-Prefix` works without this. |
| dcgm-exporter.enabled | bool | `true`                                                                             |  |
| dcgm-exporter.nodeSelector.gpu | string | `"on"`                                                                             |  |
| dcgm-exporter.serviceMonitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| dcgm-exporter.serviceMonitor.enabled | bool | `true`                                                                             |  |
| dcgm-exporter.serviceMonitor.honorLabels | bool | `false`                                                                            |  |
| dcgm-exporter.serviceMonitor.interval | string | `"15s"`                                                                            |  |
| externalPrometheus.address | string | `"http://prometheus-kube-prometheus-prometheus.prometheus.svc.cluster.local:9090"` |  |
| externalPrometheus.enabled | bool | `false`                                                                            |  |
| fullnameOverride | string | `""`                                                                               |  |
| hamiServiceMonitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| hamiServiceMonitor.enabled | bool | `true`                                                                             |  |
| hamiServiceMonitor.honorLabels | bool | `false`                                                                            |  |
| hamiServiceMonitor.interval | string | `"15s"`                                                                            |  |
| hamiServiceMonitor.relabelings | list | `[]`                                                                               |  |
| hamiServiceMonitor.svcNamespace | string | "kube-system"                                                                      | Default is "kube-system", but it should be set according to the namespace where the HAMi components are installed. || image.backend.pullPolicy | string | `"IfNotPresent"` |  |
| image.backend.repository | string | `"projecthami/hami-webui-be-oss"`                                                  |  |
| image.backend.tag | string | `"v1.0.0"`                                                                         |  |
| image.frontend.pullPolicy | string | `"IfNotPresent"`                                                                   |  |
| image.frontend.repository | string | `"projecthami/hami-webui-fe-oss"`                                                  |  |
| image.frontend.tag | string | `"v1.0.0"`                                                                         |  |
| imagePullSecrets | list | `[]`                                                                               |  |
| ingress.annotations | object | `{}`                                                                               |  |
| ingress.className | string | `""`                                                                               |  |
| ingress.enabled | bool | `false`                                                                            |  |
| ingress.hosts[0].host | string | `"chart-example.local"`                                                            |  |
| ingress.hosts[0].paths[0].path | string | `"/"`                                                                              |  |
| ingress.hosts[0].paths[0].pathType | string | `"ImplementationSpecific"`                                                         |  |
| ingress.tls | list | `[]`                                                                               |  |
| kube-prometheus-stack.alertmanager.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.crds.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.defaultRules.create | bool | `false`                                                                            |  |
| kube-prometheus-stack.enabled | bool | `true`                                                                             |  |
| kube-prometheus-stack.grafana.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.kubernetesServiceMonitors.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.nodeExporter.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.prometheus.prometheusSpec.serviceMonitorSelector.matchLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| kube-prometheus-stack.prometheusOperator.enabled | bool | `false`                                                                            |  |
| nameOverride | string | `""`                                                                               |  |
| namespaceOverride | string | `""`                                                                               |  |
| nodeSelector | object | `{}`                                                                               |  |
| podAnnotations | object | `{}`                                                                               |  |
| podSecurityContext | object | `{}`                                                                               |  |
| replicaCount | int | `1`                                                                                |  |
| resources.backend.limits.cpu | string | `"50m"`                                                                            |  |
| resources.backend.limits.memory | string | `"250Mi"`                                                                          |  |
| resources.backend.requests.cpu | string | `"50m"`                                                                            |  |
| resources.backend.requests.memory | string | `"250Mi"`                                                                          |  |
| resources.frontend.limits.cpu | string | `"200m"`                                                                           |  |
| resources.frontend.limits.memory | string | `"500Mi"`                                                                          |  |
| resources.frontend.requests.cpu | string | `"200m"`                                                                           |  |
| resources.frontend.requests.memory | string | `"500Mi"`                                                                          |  |
| securityContext | object | `{}`                                                                               |  |
| service.port | int | `3000`                                                                             |  |
| service.type | string | `"ClusterIP"`                                                                      |  |
| serviceAccount.annotations | object | `{}`                                                                               |  |
| serviceAccount.create | bool | `true`                                                                             |  |
| serviceAccount.name | string | `""`                                                                               |  |
| serviceMonitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| serviceMonitor.enabled | bool | `true`                                                                             |  |
| serviceMonitor.honorLabels | bool | `false`                                                                            |  |
| serviceMonitor.interval | string | `"15s"`                                                                            |  |
| serviceMonitor.relabelings | list | `[]`                                                                               |  |
| tolerations | list | `[]`                                                                               |  |

## Configuration Guide for HAMi-WebUI Helm Chart

### 1. About `dcgm-exporter`

If `dcgm-exporter` is already installed in your cluster, you should disable it by modifying the following setting:

```yaml
dcgm-exporter:
  enabled: false
```
This ensures that the existing `dcgm-exporter` instance is used, preventing conflicts.


### 2. About `Prometheus`

#### Scenario 1: If an existing Prometheus is available in your cluster

If your cluster already has a working Prometheus instance, you can enable the external Prometheus configuration and provide the correct address:

```yaml
externalPrometheus:
  enabled: true
  address: "<your-prometheus-address>"
```

Here, replace <your-prometheus-address> with the actual domain or internal Ingress address for your Prometheus instance.

#### Scenario 2: If no Prometheus or Operator is installed in the cluster

If there is no existing Prometheus or Prometheus Operator in your cluster, you can enable the kube-prometheus-stack to deploy Prometheus:

```yaml
kube-prometheus-stack:
  enabled: true
  crds:
    enabled: true
  ...
  prometheusOperator:
    enabled: true
  ...
```

#### Scenario 3: If Prometheus and Operator already exist, but a separate Prometheus instance is needed
If your cluster has Prometheus and Prometheus Operator, but you want to use a separate instance without affecting the existing setup, modify the configuration as follows:

```yaml
kube-prometheus-stack:
  enabled: true
  ...
```
This allows you to reuse the existing Operator and CRDs while deploying a new Prometheus instance.

### 3. Serving under a URL sub-path (base path)

By default the WebUI is served at the site root (`/`). To serve it behind a
reverse-proxy prefix such as `https://host/gpu-ui/` — without rebuilding the
frontend image — set the base path. It is resolved at **request time**, so the
same image works at any path.

There are two supported reverse-proxy modes:

**Mode A — path-stripping proxy (recommended, zero chart config).**
If your proxy strips the prefix before forwarding and sets the
`X-Forwarded-Prefix` header (nginx `proxy_set_header X-Forwarded-Prefix /gpu-ui;`,
Traefik `stripPrefix` + headers middleware, etc.), the WebUI picks the prefix up
from the header automatically. Leave `basePath` at its default `/`.

**Mode B — proxy passes the full prefixed path through (no stripping).**
Set the chart value so the BFF knows its own prefix:

```yaml
basePath: "/gpu-ui/"
# If you also expose it through this chart's ingress, point the path at the same prefix:
ingress:
  enabled: true
  hosts:
    - host: your-host
      paths:
        - path: /gpu-ui
          pathType: Prefix
```

`basePath` is injected into the frontend BFF container as the
`HAMI_WEBUI_BASE_PATH` environment variable. Values are normalized to a
leading/trailing-slash form (`gpu-ui` → `/gpu-ui/`); `/` (the default) means
root serving and preserves the historical behaviour exactly. The header (Mode A)
takes precedence over the env var when both are present.

No changes are needed on the Go API backend — it is always reached via the BFF's
`/api/vgpu` proxy.

### 4. About `jobRelease` Labels

If deploying a completely new Prometheus, you can leave the default `jobRelease: hami-webui-prometheus` unchanged.

***However, if you are integrating with an existing Prometheus instance and modifying the `prometheusSpec.serviceMonitorSelector.matchLabels`, ensure that **all** corresponding `...ServiceMonitor.additionalLabels` are updated to reflect the correct label.***

For example, if you modify:

```yaml
prometheus:
  prometheusSpec:
    serviceMonitorSelector:
      matchLabels:
        <jobRelease-label-key>: <jobRelease-label-value>
```

You must also modify all ...ServiceMonitor.additionalLabels in your values.yaml file to match:

```yaml
...ServiceMonitor:
  additionalLabels:
    <jobRelease-label-key>: <jobRelease-label-value>
```

This ensures that Prometheus will correctly discover all the ServiceMonitor configurations based on the updated labels.