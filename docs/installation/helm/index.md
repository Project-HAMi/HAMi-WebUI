# Deploy HAMi-WebUI using Helm Charts

This topic includes instructions for installing and running HAMi-WebUI on Kubernetes using Helm Charts.

[Helm](https://helm.sh/) is an open-source command line tool used for managing Kubernetes applications. It is a graduate project in the [CNCF Landscape](https://www.cncf.io/projects/helm/).

The HAMi-WebUI open-source community offers Helm Charts for running it on Kubernetes. Please be aware that the code is provided without any warranties. If you encounter any problems, you can report them to the [Official GitHub repository](https://github.com/hami-webui/helm-charts/).

## Before you begin

To install HAMi-WebUI using Helm, ensure you have completed the following:

- Install a Kubernetes server on your machine. For information about installing Kubernetes, refer to [Install Kubernetes](https://kubernetes.io/docs/setup/).

- Install the latest stable version of Helm. For information on installing Helm, refer to [Install Helm](https://helm.sh/docs/intro/install/).

- Install HAMi on your Kubernetes cluster. For information about installing HAMi, refer to [Install HAMi](https://github.com/Project-HAMi/HAMi?tab=readme-ov-file#quick-start).

## Install HAMi-WebUI using Helm

When you install HAMi-WebUI using Helm, you complete the following tasks:

1. Set up the HAMi-WebUI Helm repository, which provides a space in which you will install HAMi-WebUI.

2. Deploy HAMi-WebUI using Helm, which installs HAMi-WebUI into a namespace.

3. Access HAMi-WebUI by navigating to the provided URL.

### Set up the HAMi-WebUI Helm repository

To set up the HAMi-WebUI Helm repository so that you download the correct HAMi-WebUI Helm charts on your machine, complete the following steps:

1. To add the HAMi-WebUI repository, use the following command syntax:

   `helm repo add <DESIRED-NAME> <HELM-REPO-URL>`

   The following example adds the `hami-webui` Helm repository.

   ```bash
   helm repo add hami-webui https://project-hami.github.io/HAMi-WebUI
   ```

2. Run the following command to verify the repository was added:

   ```bash
   helm repo list | grep hami-webui
   ```

   After you add the repository, you should see an output similar to the following:

   ```bash
   hami-webui  https://project-hami.github.io/HAMi-WebUI
   ```

3. Run the following command to update the repository to download the latest HAMi-WebUI Helm charts:

   ```bash
   helm repo update
   ```

### Deploy the HAMi-WebUI Helm charts

After you have set up the HAMi-WebUI Helm repository, you can start to deploy it on your Kubernetes cluster.

When you deploy HAMi-WebUI Helm charts, use a separate namespace instead of relying on the default namespace. The default namespace might already have other applications running, which can lead to conflicts and other potential issues.

When you create a new namespace in Kubernetes, you can better organize, allocate, and manage cluster resources. For more information, refer to [Namespaces](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/).

1. To create a namespace, run the following command:

   ```bash
   kubectl create namespace hami
   ```

   You will see an output similar to this, which means that the namespace has been successfully created:

   ```bash
   namespace/hami created
   ```

2. Search for the official `hami-webui/hami-webui` repository using the command:

   `helm search repo <repo-name/package-name>`

   For example, the following command provides a list of the HAMi-WebUI Helm Charts from which you will install the latest version of the HAMi-WebUI chart.

   ```bash
   helm search repo hami-webui/hami-webui
   ```

3. Before deploying, ensure that you configure the `values.yaml` file to match your cluster’s requirements. For detailed instructions, refer to the [Configuration Guide for HAMi-WebUI Helm Chart](../../../charts/hami-webui/README.md#configuration-guide-for-hamiwebui-helm-chart)
   > _**Important**_: You must adjust the values.yaml before proceeding with the deployment.

   Download the `values.yaml` file from the Helm Charts repository:

   https://github.com/Project-HAMi/HAMi-WebUI/blob/main/charts/hami-webui/values.yaml

4. Once you've adjusted the `values.yaml`, run the following command to deploy the HAMi-WebUI Helm Chart inside your namespace:

   ```bash
   helm install my-hami-webui hami-webui/hami-webui --namespace hami -f values.yaml
   ```

   Where:

    - `helm install`: Installs the chart by deploying it on the Kubernetes cluster
    - `my-hami-webui`: The logical chart name that you provided
    - `hami-charts/hami-webui`: The repository and package name to install
    - `--namespace`: The Kubernetes namespace (i.e. `hami`) where you want to deploy the chart

5. To verify the deployment status, run the following command and verify that `deployed` appears in the **STATUS** column:

   ```bash
   helm list -n hami
   ```

   You should see an output similar to the following:

   ```bash
   NAME            NAMESPACE       REVISION        UPDATED                                 STATUS          CHART                   APP VERSION
   my-hami-webui   hami            1               2024-09-11 14:19:09.003195 +0800 CST    deployed        hami-webui-1.1.0        1.1.0
   ```

6. To check the overall status of all the objects in the namespace, run the following command:

   ```bash
   kubectl get all -n hami
   ```

   If you encounter errors or warnings in the **STATUS** column, check the logs and refer to the Troubleshooting section of this documentation.

### Access HAMi-WebUI

1. Run the following command to do a port-forwarding of the HAMi-WebUI service on port `3000`.

   ```bash
   kubectl port-forward service/my-hami-webui 3000:3000 --namespace=hami
   ```

   For more information about port-forwarding, refer to [Use Port Forwarding to Access Applications in a Cluster](https://kubernetes.io/docs/tasks/access-application-cluster/port-forward-access-application-cluster/).

2. Navigate to `localhost:3000` in your browser.

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