# Deploy HAMi-WebUI using Helm Charts

This topic includes instructions for installing and running HAMi-WebUI on Kubernetes using Helm Charts.

The WebUI can only be accessed by your localhost, so you need to connect your localhost to the cluster by configuring `~/.kube/config` 

[Helm](https://helm.sh/) is an open-source command line tool used for managing Kubernetes applications. It is a graduate project in the [CNCF Landscape](https://www.cncf.io/projects/helm/).

The HAMi-WebUI open-source community offers Helm Charts for running it on Kubernetes. Please be aware that the code is provided without any warranties. If you encounter any problems, you can report them to the [Official GitHub repository](https://github.com/hami-webui/helm-charts/).

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

   You should get the expected both 'hami-webui' and 'hami-webui-dcgm-exporter' in running state if installation is successful.

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

### Backend service boundary

HAMi-WebUI exposes the supported browser API through the same-origin Web entry
on the primary Service. The backend listener on port `8000` also serves raw API,
`/readyz`, `/metrics`, and Swagger endpoints, so the chart creates a separate
ClusterIP Service with a generated `*-backend` name (for example,
`my-hami-webui-backend`) for backend discovery and Prometheus scraping.

For Chart 1.x upgrade compatibility, the primary Service still includes port
`8000` by default. The port is deprecated and can be removed from the primary
Service without affecting WebUI traffic or the included ServiceMonitor:

```yaml
service:
  legacyBackendPort: false
```

Set this to `false` for `NodePort` and `LoadBalancer` Services unless an existing
integration still connects directly to the raw backend. Move in-cluster clients
to the generated backend Service on port `8000`. For local diagnostics, forward
that Service instead of changing the primary Service type:

```bash
kubectl port-forward service/my-hami-webui-backend 8000:8000 --namespace=kube-system
```

Use NetworkPolicy or an equivalent cluster policy when backend access must also
be restricted inside the cluster; a ClusterIP Service is a discovery boundary,
not an authorization boundary.

The included ServiceMonitor now selects only the backend Service, so retaining
the compatibility port does not double-scrape Pods. It preserves the existing
Prometheus `job` label through an explicit Service label; the generated `service`
target label changes to the generated backend Service name. Update custom rules
that match `service` before upgrading.

Custom ServiceMonitors that still select the primary Service—whether by
`component: hami-webui` or only the common name and instance labels—and port
`metrics` must move to `component: backend` and port `backend-http`. Otherwise
they can duplicate the included ServiceMonitor while the compatibility port is
enabled, and stop finding a target when it is disabled. The compatibility port
will be removed from the primary Service in Chart 2.0.0.

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
kubectl logs --namespace=hami deploy/my-hami-webui -c hami-webui-fe-oss
kubectl logs --namespace=hami deploy/my-hami-webui -c hami-webui-be-oss
```

For more information about accessing Kubernetes application logs, refer to [Pods](https://kubernetes.io/docs/reference/kubectl/cheatsheet/#interacting-with-running-pods) and [Deployments](https://kubernetes.io/docs/reference/kubectl/cheatsheet/#interacting-with-deployments-and-services).


## Uninstall the HAMi-WebUI deployment

To uninstall the HAMi-WebUI deployment, run the command:

`helm uninstall <RELEASE-NAME> <NAMESPACE-NAME>`

```bash
helm uninstall my-hami-webui -n hami
```

This deletes all of the objects from the given namespace hami.

If you want to delete the namespace `hami`, then run the command:

```bash
kubectl delete namespace hami
```
