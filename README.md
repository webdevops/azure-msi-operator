Operator for Azure Managed Service Identity in Kubernetes
=========================================================

[![license](https://img.shields.io/github/license/webdevops/azure-msi-operator.svg)](https://github.com/webdevops/azure-msi-operator/blob/master/LICENSE)
[![Docker](https://img.shields.io/badge/docker-webdevops%2Fazure--msi--operator-blue.svg?longCache=true&style=flat&logo=docker)](https://hub.docker.com/r/webdevops/azure-msi-operator/)
[![Docker Build Status](https://img.shields.io/docker/build/webdevops/azure-msi-operator.svg)](https://hub.docker.com/r/webdevops/azure-msi-operator/)

Operator for Azure Managed Service Identity (MSI) in Kubernetes, requies [Azure aaa-pod-idenity service](https://github.com/Azure/aad-pod-identity)
Finds and securly creates `AzureIdentity` resources in Kubernetes automatically when found in Azure:

Example Azure MSI:
```yaml
ResourceName: foobar
ClientID: df398181-f42f-41b4-b791-b1d4572be315

```

Creates Kubernetes resource:
```yaml
apiVersion: "aadpodidentity.k8s.io/v1"
kind: AzureIdentity
metadata:
  name: foobar-df398181-f42f-41b4-b791-b1d4572be315
spec:
  type: 0
  resourceID: /subscriptions/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx/resourcegroups/resourcegroup-name/providers/Microsoft.ManagedIdentity/userAssignedIdentities/foobar
  clientID: df398181-f42f-41b4-b791-b1d4572be315
```

Configuration
-------------

```
Usage:
  azure-msi-operator [OPTIONS]

Application Options:
  -v, --verbose                    Verbose mode [$VERBOSE]
      --sync.interval=             Sync interval (time.duration) (default: 1h) [$SYNC_INTERVAL]
      --azure.subscription=        Azure subscription ID [$AZURE_SUBSCRIPTION_ID]
      --kubeconfig=                Kuberentes config path (should be empty if in-cluster) [$KUBECONFIG]
      --msi.scheme.group=          MSI scheme group name (default: aadpodidentity.k8s.io) [$MSI_SCHEME_GROUP]
      --msi.scheme.version=        MSI scheme version (default: v1) [$MSI_SCHEME_VERSION]
      --msi.scheme.resource=       MSI scheme resource name (singular) (default: AzureIdentity) [$MSI_SCHEME_RESOURCE]
      --msi.scheme.resources=      MSI scheme resources name (pural) (default: azureidentities) [$MSI_SCHEME_RESOURCES]
      --msi.namespaced             Set aadpodidentity.k8s.io/Behavior=namespaced annotation [$MSI_NAMESPACED]
      --msi.tag.namespace=         Name of Kubernetes namespace (default: k8snamespace) [$MSI_TAG_NAMESPACE]
      --msi.template.resourcename= Golang template for Kubernetes resource name (default: {{ .Name }}-{{ .ClientId }}) [$MSI_TEMPLATE_RESOURCENAME]
      --bind=                      Server address (default: :8080) [$SERVER_BIND]

Help Options:
  -h, --help                       Show this help message
```

for Azure API authentication (using ENV vars) see https://github.com/Azure/azure-sdk-for-go#authentication


Metrics
-------

| Metric                                         | Type         | Description                                                                           |
|------------------------------------------------|--------------|---------------------------------------------------------------------------------------|
| `azuremsi_sync_time`                           | Gauge        | Time (unix timestamp) of last sync run per Azure Subscription                         |
| `azuremsi_sync_duration`                       | Gauge        | Duration of last sync per Azure Subscription                                          |
| `azuremsi_sync_resources_errors`               | Counter      | Number of errors while syncing                                                        |
| `azuremsi_sync_resources_success`              | Counter      | Number of successfull syncs                                                           |
