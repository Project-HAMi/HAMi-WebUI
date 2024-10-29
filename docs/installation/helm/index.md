# Deploy HAMi-WebUI using Helm Charts

This topic includes instructions for installing and running HAMi-WebUI on Kubernetes using Helm Charts.

The WebUI can only be accessed by your localhost, so you need to connect your localhost to the cluster by configuring `~/.kube/config` 

[Helm](https://helm.sh/) is an open-source command line tool used for managing Kubernetes applications. It is a graduate project in the [CNCF Landscape](https://www.cncf.io/projects/helm/).

The HAMi-WebUI open-source community offers Helm Charts for running it on Kubernetes. Please be aware that the code is provided without any warranties. If you encounter any problems, you can report them to the [Official GitHub repository](https://github.com/hami-webui/helm-charts/).

## Prequisities

To install HAMi-WebUI using Helm, ensure you meet these requirements:

1. Kubectl on your localhost

2. [HAMi](https://github.com/Project-HAMi/HAMi?tab=readme-ov-file#quick-start) >= 2.4.0

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

### Access HAMi-WebUI

1. Configure ~/.kube/config in your localhost to be able to connect your cluster.


2. Run the following command to do a port-forwarding of the HAMi-WebUI service on port `3000` in your localhost.

   ```bash
   kubectl port-forward service/my-hami-webui 3000:3000 --namespace=kube-system
   ```

   For more information about port-forwarding, refer to [Use Port Forwarding to Access Applications in a Cluster](https://kubernetes.io/docs/tasks/access-application-cluster/port-forward-access-application-cluster/).

3. Navigate to `localhost:3000` in your browser.

   The HAMi-WebUI resources-overview page appears.

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