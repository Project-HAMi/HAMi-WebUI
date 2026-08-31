# Deploy HAMi-WebUI using Helm Charts

This topic includes instructions for installing and running HAMi-WebUI on Kubernetes using Helm Charts.

The examples below use `kubectl port-forward` for local access. Configure
`~/.kube/config` so `kubectl` and Helm can reach the target cluster.

[Helm](https://helm.sh/) is an open-source command line tool used for managing Kubernetes applications. It is a graduate project in the [CNCF Landscape](https://www.cncf.io/projects/helm/).

The HAMi-WebUI community publishes a Helm chart for Kubernetes. Report problems
in the [HAMi-WebUI repository](https://github.com/Project-HAMi/HAMi-WebUI/issues).

> This page follows the Chart source in the current Git branch. The published
> Helm repository may contain an older release. Always select a Chart version
> explicitly and use the values packaged with that same version;
> `values.yaml` from `main` is not compatible with every released Chart.

## Prerequisites

To install HAMi-WebUI using Helm, ensure you meet these requirements:

1. Kubectl on your localhost

2. [HAMi](https://github.com/Project-HAMi/HAMi?tab=readme-ov-file#quick-start) (see version compatibility below)

### Version compatibility

> _**Important**_: HAMi-WebUI v1.2.0+ uses the HAMi 2.9.0 metrics schema (renamed metrics/labels). If you upgrade HAMi-WebUI without upgrading HAMi, dashboards may break.

| HAMi-WebUI version | Supported HAMi version | Metrics schema | Notes |
| --- | --- | --- | --- |
| <= v1.1.0 | >= 2.4.0, < 2.9.0 | old labels: `deviceuuid`, `devicetype`, `podnamespace`, `podname`, `ctrname` | For existing HAMi deployments before the metrics rename |
| v1.2.0+ | >= 2.9.0 | new labels: `device_uuid`, `device_type`, `namespace`, `pod`, `container` | Required after the HAMi 2.9.0 metrics rename |

3. External mode requires a reachable Prometheus-compatible HTTP API
   (Prometheus > 2.8.0 or VictoriaMetrics). Using the Chart's `ServiceMonitor`
   resources also requires a running Prometheus Operator and its CRD; the raw
   scrape path documented below does not. Self-contained mode provisions the
   API, Operator, and CRDs together.

4. Kubernetes >= 1.19 and Helm > 3.0. The Kubernetes minimum matches the
   bundled monitoring dependencies and guarantees the stable Ingress
   `pathType: Prefix` contract.

## Install HAMi-WebUI using Helm

### Deploy the HAMi-WebUI Helm charts

To set up the HAMi-WebUI Helm repository so that you download the correct HAMi-WebUI Helm charts on your machine, complete the following steps:

1. To add the HAMi-WebUI repository, use the following command syntax:

   ```bash
   helm repo add hami-webui https://project-hami.github.io/HAMi-WebUI
   helm repo update
   helm search repo hami-webui/hami-webui --versions
   ```

2. Select the release you want to install, set its version, and retrieve the
   defaults from that exact package:

   ```bash
   # Replace X.Y.Z with a version listed by helm search.
   CHART_VERSION="X.Y.Z"
   helm show values hami-webui/hami-webui \
     --version "${CHART_VERSION}" > values.yaml
   ```

   Review and edit `values.yaml` for your cluster, then install the same version:

   ```bash
   : "${CHART_VERSION:?Set CHART_VERSION to the version used to generate values.yaml}"
   PROMETHEUS_ADDRESS="http://<prometheus-service>.<namespace>.svc.cluster.local:9090"
   PROMETHEUS_RELEASE="<kube-prometheus-stack-release>"
   helm install my-hami-webui hami-webui/hami-webui \
     --version "${CHART_VERSION}" \
     --namespace kube-system \
     --values values.yaml \
     --set externalPrometheus.enabled=true \
     --set-string externalPrometheus.address="${PROMETHEUS_ADDRESS}" \
     --set-string serviceMonitor.additionalLabels.release="${PROMETHEUS_RELEASE}" \
     --set-string hamiServiceMonitor.additionalLabels.release="${PROMETHEUS_RELEASE}" \
     --set-string dcgm-exporter.serviceMonitor.additionalLabels.release="${PROMETHEUS_RELEASE}"
   ```

   Replace both placeholders. This example uses an existing
   kube-prometheus-stack release whose default selector is
   `release=<Helm release>`. If your Prometheus uses another selector, apply its
   labels to all three ServiceMonitors instead. Its
   `serviceMonitorNamespaceSelector` must include the HAMi-WebUI namespace.
   Do not copy the `main` values file into an older Chart release.

3. Verify both the workload and the scrape contract:

   ```bash
   kubectl get pods -n kube-system | grep webui
   kubectl get servicemonitor -n kube-system \
     -l app.kubernetes.io/instance=my-hami-webui
   ```

   A successful Chart 2 installation has one Ready HAMi-WebUI Pod;
   every HAMi-WebUI Pod contains one application container. When the bundled
   dcgm-exporter is enabled, its DaemonSet schedules Pods on nodes matching its
   node selector. Pod readiness does not prove that an external Prometheus has
   selected or scraped these ServiceMonitors; use the query checks in
   [Prometheus scrape configuration](#prometheus-scrape-configuration).

### Open HAMi-WebUI

Forward the primary Service to your workstation:

```bash
kubectl port-forward service/my-hami-webui 3000:http --namespace=kube-system
```

For more information, see Kubernetes
[Use Port Forwarding to Access Applications in a Cluster](https://kubernetes.io/docs/tasks/access-application-cluster/port-forward-access-application-cluster/).

Open `http://localhost:3000/` in a browser. If `frontend.basePath` is
configured, append that normalized path instead; the `helm install` and
`helm get notes` output prints the exact URL.

The HAMi-WebUI resource overview appears.

### Upgrade from Chart 1.3 to 2.0

Chart 2 replaces the Chart 1.x frontend/backend image pair with one application
image and one container. This is an intentional major-version boundary: do not
pass a Chart 1.x values file to Chart 2 and do not use `--reuse-values`.

First save the values and revision used by the current release:

```bash
helm get values my-hami-webui --namespace kube-system --output yaml > values-v1-backup.yaml
helm history my-hami-webui --namespace kube-system
```

Create a new `values-v2.yaml` from the Chart 2 defaults, then copy only the
settings you still need. Chart 2 supports external Prometheus authentication
and TLS settings, ServiceMonitor labels, Ingress, scheduling, security contexts,
`frontend.basePath`, and `frontend.frameAncestors`; copy only settings that are
actually present in your deployment.

Do not copy these Chart 1.x settings:

- `image.frontend` or `image.backend`; configure the flat `image` value;
- `resources.frontend` or `resources.backend`; choose one `resources` budget;
- `env.frontend` or `env.backend`; use the single `env` list;
- container-specific probes; use the top-level `probes` settings; or
- `frontend.proxyTimeout`, `backend.grpc`, or `service.legacyBackendPort`, which
  no longer apply to the in-process API and single-container Service topology.

After Chart 2.0.0 is published, upgrade with Helm defaults reset so removed
values cannot leak from release history:

```bash
helm upgrade my-hami-webui hami-webui/hami-webui \
  --namespace kube-system \
  --version 2.0.0 \
  --reset-values \
  --values values-v2.yaml \
  --wait
```

Chart 2 rejects known Chart 1.x value shapes instead of silently ignoring them.
If rollback is needed, restore the complete previous Helm revision—not an old
frontend or backend image inside the Chart 2 Pod:

```bash
helm rollback my-hami-webui <CHART-1-REVISION> \
  --namespace kube-system \
  --wait
```

That rollback restores the Chart 1.3 templates, values, and two-image runtime
together. Use `--reset-values` again when moving forward to Chart 2.

### Select a Prometheus ownership mode

Chart 2 requires exactly one Prometheus mode. Helm rejects an installation when
neither mode is configured, when both are enabled, or when external mode has no
absolute HTTP(S) address. This prevents a healthy-looking Pod from querying a
guessed Service that the Chart never created.

The recommended production path is an independently managed Prometheus and
Prometheus Operator. Enable `externalPrometheus` as shown in the installation
command, then make the Operator's Prometheus resource select all three generated
ServiceMonitors and their namespaces. Keep this Chart's `hamiServiceMonitor`
enabled and leave the HAMi Chart's `prometheus.enabled=false` default: this
monitor sets `honorLabels: true`. Disable it only when another selected monitor
owns the same endpoint and is confirmed to preserve those workload labels.

For a self-contained evaluation cluster with no existing Prometheus or
Operator, use:

```yaml
externalPrometheus:
  enabled: false
kube-prometheus-stack:
  enabled: true
  crds:
    enabled: true
  prometheusOperator:
    enabled: true
```

This mode installs cluster-scoped CRDs and an Operator as part of the WebUI
release. Helm does not upgrade or delete CRDs as ordinary release resources, so
manage the monitoring stack as a separate release for long-lived production
clusters.

If a compatible Operator and CRDs already exist but a separate Prometheus
instance is desired, enable only `kube-prometheus-stack.enabled`; the Chart's
defaults deliberately leave that dependency's CRDs and Operator disabled. The
existing Operator must watch the HAMi-WebUI namespace.

### Authenticate to an external Prometheus

Use an existing Kubernetes Secret in the HAMi-WebUI namespace. The Chart does
not accept inline credentials and does not copy Secret values, names, or data
keys into its ConfigMap or Pod environment. It mounts only the selected keys at
fixed file paths. The examples read credentials from local files so their values
do not become command-line arguments or shell history.

For a bearer token or another HTTP Authorization scheme, create the Secret and
select its data key:

```bash
kubectl create secret generic hami-webui-prometheus-auth \
  --namespace kube-system \
  --from-file=token=/secure/path/prometheus-token
```

```yaml
externalPrometheus:
  enabled: true
  address: "https://prometheus.example.com"
  authorization:
    type: Bearer
    existingSecret: hami-webui-prometheus-auth
    credentialsKey: token
```

For HTTP Basic Authentication, use one Secret containing both fields:

```bash
kubectl create secret generic hami-webui-prometheus-basic-auth \
  --namespace kube-system \
  --from-file=username=/secure/path/prometheus-username \
  --from-file=password=/secure/path/prometheus-password
```

```yaml
externalPrometheus:
  enabled: true
  address: "https://prometheus.example.com"
  basicAuth:
    existingSecret: hami-webui-prometheus-basic-auth
    usernameKey: username
    passwordKey: password
```

`authorization` and `basicAuth` are mutually exclusive. The Chart rejects
credentials embedded in `externalPrometheus.address` such as
`https://user:password@prometheus.example.com`; URL credentials are readily
exposed by configuration output and logs. Basic Authentication and bearer
tokens should use HTTPS unless transport security is provided elsewhere.
Authenticated redirects are followed only when the scheme, host and effective
port remain the same; cross-origin redirects fail before a request is sent to
the new origin.

The Prometheus client reads these files for every request. After Kubernetes
propagates a Secret update to the mounted volume, new requests use the
rotated credentials without restarting the Pod. The Chart deliberately mounts
the directory rather than using `subPath`, because a `subPath` mount does not
receive automated Secret updates.

The direct server configuration field `prometheus.auth` remains available for
pre-2.0 compatibility but is deprecated. New direct configurations should use
`prometheus.authorization.credentials_file` or
`prometheus.basic_auth.username_file` and `password_file`; these modes are
mutually exclusive with the legacy field.

### HTTPS external Prometheus

HAMi-WebUI verifies HTTPS certificates with the container's system trust store
by default. No TLS values are required for a publicly trusted endpoint.

For a private CA, create a Secret in the HAMi-WebUI namespace and reference its
data key:

```bash
kubectl create secret generic hami-webui-prometheus-tls \
  --namespace kube-system \
  --from-file=ca.crt=/path/to/prometheus-ca.crt
```

```yaml
externalPrometheus:
  enabled: true
  address: "https://prometheus.example.com"
  tls:
    existingSecret: hami-webui-prometheus-tls
    caKey: ca.crt
```

When Prometheus requires mutual TLS, add the client certificate and key to the
same Secret and configure `certKey` and `keyKey` together. `serverName` is
available when certificate verification must use a name different from the URL
host. Secret contents are mounted only into the HAMi-WebUI application
container; they are not copied into the rendered ConfigMap.

`insecureSkipVerify: true` remains available only as an explicit temporary
escape hatch. It disables certificate-chain and host-name verification and
should not replace configuring the correct CA.

HAMi-WebUI 2.0.0 enables certificate verification by default; earlier versions
silently skipped it for every HTTPS Prometheus endpoint. Before upgrading an
installation that relies on an untrusted self-signed certificate, configure its
CA as shown above or explicitly opt into the temporary insecure setting.

### Prometheus scrape configuration

ServiceMonitor objects are discovery requests, not proof of collection. The
running Prometheus resource must select their labels through
`serviceMonitorSelector` and must select the HAMi-WebUI namespace through
`serviceMonitorNamespaceSelector`. A common kube-prometheus-stack installation
selects `release=<its Helm release>`; the first-install command above applies
that label to all three generated ServiceMonitors.

HAMi's vgpu-monitor exposes workload identity as `namespace`, `pod`, and
`container`. When Prometheus adds scrape-target labels with the same names and
source labels are not honored, it renames the workload labels to `exported_*`.
HAMi-WebUI's queries then return no series, and task utilization is reported as
`0`.

The included ServiceMonitor sets `hamiServiceMonitor.honorLabels: true`. If an
external Prometheus discovers this ServiceMonitor, no additional scrape job is
needed. For a separately managed scrape, set `honorLabels: true` on its
ServiceMonitor or use the equivalent raw Prometheus setting:

```yaml
scrape_configs:
  - job_name: hami-device-plugin-monitor
    honor_labels: true
    # ...
```

`Prometheus.spec.overrideHonorLabels: true` forces
`honorLabels` off for every ServiceMonitor, including this one.

If the same Prometheus selects both this Chart's monitor and HAMi's built-in
device-plugin ServiceMonitor, the endpoint is scraped twice. The HAMi monitor
does not currently set `honorLabels: true`, so prefer this Chart's monitor and
leave HAMi's `prometheus.enabled=false`. Use another owner only after confirming
that its monitor preserves the workload labels, and select exactly one.

For a Prometheus installation without the Operator, keep external mode enabled
but turn off every Operator custom resource:

```yaml
externalPrometheus:
  enabled: true
  address: "http://<prometheus-address>:9090"
serviceMonitor:
  enabled: false
hamiServiceMonitor:
  enabled: false
dcgm-exporter:
  serviceMonitor:
    enabled: false
```

The manually managed scrape configuration must collect all of these targets:

- the generated `my-hami-webui-backend` Service on port `8000`, path `/metrics`;
- HAMi's device-plugin monitor Service on `monitorport`, path `/metrics`, with
  `honor_labels: true`;
- kube-state-metrics for Kubernetes CPU and memory capacity; and
- every vendor exporter or monitor used by the enabled hardware providers. The
  consumed metric families include `DCGM_*` for NVIDIA, `npu_*` for Ascend,
  `dcu_*`/`vdcu_*` for DCU, `mlu_*` for MLU, and `mx_*` for MetaX.

Service names and discovery labels depend on the releases already installed in
the cluster, so the Chart cannot safely write this external configuration. Raw
scraping is therefore an advanced, user-managed path.

After installation, query the selected Prometheus rather than relying only on
Pod readiness. At minimum verify:

```promql
hami_vgpu_count or hami_core_size
kube_node_status_allocatable
```

Then query a metric that matches the installed hardware:

| Hardware | Example query |
| --- | --- |
| NVIDIA | `DCGM_FI_DEV_GPU_UTIL` |
| Ascend | `npu_chip_info_utilization` |
| DCU | `dcu_utilizationrate` |
| MLU | `mlu_utilization` |
| MetaX | `mx_gpu_usage` |

For NVIDIA, also query `hami_container_device_utilization_ratio` while a HAMi
workload is active. It is NVIDIA container telemetry, not a cross-vendor
installation check. Missing applicable series means the address, selector,
namespace selector, scrape ownership, or vendor exporter is still incomplete.

### Single-container service boundary

Chart 2 runs the SPA, browser API, metrics collector, and Kubernetes providers
in one Go process and one container. The process keeps two listeners:

- port `3000` is exposed by the primary Service and is the supported browser
  entry for the SPA and `/api/vgpu/v1/*`;
- port `8000` is exposed only through the generated internal `*-backend`
  ClusterIP Service for `/readyz`, `/metrics`, and diagnostics.

The port `8000` listener also contains implementation-level API and Swagger
routes. It is not a public compatibility contract or an authorization boundary.
The primary Service no longer exposes it, and Chart 2 removes
`service.legacyBackendPort`.

For local diagnostics, forward the internal Service instead of changing the
primary Service type:

```bash
kubectl port-forward service/my-hami-webui-backend 8000:8000 --namespace=kube-system
```

ClusterIP controls discovery and exposure through the primary Service; it does
not by itself authorize callers inside the cluster.

The included ServiceMonitor selects only the internal Service and preserves the
existing Prometheus `job` label through an explicit Service label. The generated
`service` target label is the generated internal Service name. Update custom
rules that match `service` before upgrading.

Custom ServiceMonitors that still select the primary Service—whether by
`component: hami-webui` or only the common name and instance labels—and port
`metrics` must move to `component: backend` and port `backend-http`; otherwise
they no longer find a target in Chart 2.

### Serve from a URL prefix or embed in a platform

To host HAMi-WebUI below a path such as `/hami/` or embed the whole application
in an internal portal, configure the runtime base path and an explicit framing
policy. See [Serve and embed HAMi-WebUI under a URL prefix](../embedding.md).

## Troubleshooting

This section includes troubleshooting tips you might find helpful when deploying HAMi-WebUI on Kubernetes via Helm.

### Collect logs

It is important to view the HAMi-WebUI server logs while troubleshooting any issues.

To check the HAMi-WebUI logs, run the following command:

```bash
kubectl logs --namespace=kube-system deployment/my-hami-webui
```

For more information about accessing Kubernetes application logs, refer to [Pods](https://kubernetes.io/docs/reference/kubectl/cheatsheet/#interacting-with-running-pods) and [Deployments](https://kubernetes.io/docs/reference/kubectl/cheatsheet/#interacting-with-deployments-and-services).


## Uninstall the HAMi-WebUI deployment

To uninstall the HAMi-WebUI deployment, run the command:

`helm uninstall <RELEASE-NAME> --namespace <NAMESPACE-NAME>`

```bash
helm uninstall my-hami-webui --namespace kube-system
```

This deletes the objects managed by that Helm release. It does not delete the
namespace.
