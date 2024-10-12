# HAMi-WebUI

![Version: 1.0.3](https://img.shields.io/badge/Version-1.0.3-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.3](https://img.shields.io/badge/AppVersion-1.0.3-informational?style=flat-square)

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

### 3. About `jobRelease` Labels

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