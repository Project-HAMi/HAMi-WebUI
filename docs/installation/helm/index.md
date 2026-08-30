# Deploy HAMi-WebUI using Helm Charts

This topic includes instructions for installing and running HAMi-WebUI on Kubernetes using Helm Charts.

The examples below use `kubectl port-forward` for local access. Configure
`~/.kube/config` so `kubectl` and Helm can reach the target cluster.

[Helm](https://helm.sh/) is an open-source command line tool used for managing Kubernetes applications. It is a graduate project in the [CNCF Landscape](https://www.cncf.io/projects/helm/).

The HAMi-WebUI community publishes a Helm chart for Kubernetes. Report problems
in the [HAMi-WebUI repository](https://github.com/Project-HAMi/HAMi-WebUI/issues).

## Prerequisites

To install HAMi-WebUI using Helm, ensure you meet these requirements:

1. Kubectl on your localhost

2. [HAMi](https://github.com/Project-HAMi/HAMi?tab=readme-ov-file#quick-start) (see version compatibility below)

### Version compatibility

> _**Important**_: HAMi-WebUI v1.1.1+ switches to the HAMi 2.9.0 metrics schema (renamed metrics/labels). If you upgrade HAMi-WebUI without upgrading HAMi, dashboards may break.

| HAMi-WebUI version | Supported HAMi version | Metrics schema | Notes |
| --- | --- | --- | --- |
| <= v1.1.0 | >= 2.4.0, < 2.9.0 | old labels: `deviceuuid`, `devicetype`, `podnamespace`, `podname`, `ctrname` | For existing HAMi deployments before the metrics rename |
| v1.1.1+ | >= 2.9.0 | new labels: `device_uuid`, `device_type`, `namespace`, `pod`, `container` | Required after the HAMi 2.9.0 metrics rename |

3. Prometheus > 2.8.0

4. Helm > 3.0

## Install HAMi-WebUI using Helm

### Deploy the HAMi-WebUI Helm charts

To set up the HAMi-WebUI Helm repository so that you download the correct HAMi-WebUI Helm charts on your machine, complete the following steps:

1. To add the HAMi-WebUI repository, use the following command syntax:

   ```bash
   helm repo add hami-webui https://project-hami.github.io/HAMi-WebUI
   ```

2. Deploy HAMi-WebUI using following command:

   ```bash
   helm install my-hami-webui hami-webui/hami-webui --set externalPrometheus.enabled=true --set externalPrometheus.address="http://prometheus-kube-prometheus-prometheus.monitoring.svc.cluster.local:9090" -n kube-system
   ```

   > _**Important**_: You need to replace the value of 'externalPrometheus.address' to your prometheus address inside cluster

   You can set other fields in [values.yaml](https://github.com/Project-HAMi/HAMi-WebUI/blob/main/charts/hami-webui/values.yaml) during installation according to configuration [document](https://github.com/Project-HAMi/HAMi-WebUI/blob/main/charts/hami-webui/README.md#values)

3. Run the following command to verify the installation:

   ```bash
   kubectl get pods -n kube-system | grep webui
   ```

   By default, a successful Chart 2 installation has one Ready HAMi-WebUI Pod;
   every HAMi-WebUI Pod contains one application container. When the bundled
   dcgm-exporter is enabled, its DaemonSet schedules Pods on nodes matching its
   node selector.

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
settings you still need. Chart 2 supports external Prometheus and TLS settings,
ServiceMonitor labels, Ingress, scheduling, security contexts,
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

### Prometheus scrape labels

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

If the same Prometheus selects both this chart's ServiceMonitor and HAMi's
built-in device-plugin ServiceMonitor (`prometheus.enabled` in the HAMi chart),
the endpoint is scraped twice. Configure that Prometheus to select only one.

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

### Access HAMi-WebUI

1. Configure ~/.kube/config in your localhost to be able to connect your cluster.


2. Run the following command to do a port-forwarding of the HAMi-WebUI service on port `3000` in your localhost.

   ```bash
   kubectl port-forward service/my-hami-webui 3000:3000 --namespace=kube-system
   ```

   For more information about port-forwarding, refer to [Use Port Forwarding to Access Applications in a Cluster](https://kubernetes.io/docs/tasks/access-application-cluster/port-forward-access-application-cluster/).

3. Navigate to `localhost:3000` in your browser.

   The HAMi-WebUI resources-overview page appears.

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
