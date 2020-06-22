Operator for Azure Managed Service Identity in Kubernetes
=========================================================

[![license](https://img.shields.io/github/license/webdevops/azure-msi-operator.svg)](https://github.com/webdevops/azure-msi-operator/blob/master/LICENSE)
[![Docker](https://img.shields.io/badge/docker-webdevops%2Fazure--msi--operator-blue.svg?longCache=true&style=flat&logo=docker)](https://hub.docker.com/r/webdevops/azure-msi-operator/)
[![Docker Build Status](https://img.shields.io/docker/build/webdevops/azure-msi-operator.svg)](https://hub.docker.com/r/webdevops/azure-msi-operator/)

Operator for Azure Managed Service Identity (MSI) in Kubernetes, requies [Azure aad-pod-identity service](https://github.com/Azure/aad-pod-identity)
Finds and securly creates `AzureIdentity` resources in Kubernetes automatically when found in Azure:

Example Azure MSI:
```yaml
ResourceName: foobar
ResourceGroup: barfoo
Subscription: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
ClientID: df398181-f42f-41b4-b791-b1d4572be315
Tags:
    k8snamespace: test123

```

Creates Kubernetes AzureIdentity:
```yaml
apiVersion: "aadpodidentity.k8s.io/v1"
kind: AzureIdentity
metadata:
  name: foobar-df398181-f42f-41b4-b791-b1d4572be315
  namespace: test123
  labels:
    azure.k8s.io/msi-resourcename: foobar
    azure.k8s.io/msi-resourcerroup: f-we-foobar-rg
    azure.k8s.io/msi-subscription: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
spec:
  type: 0
  resourceID: /subscriptions/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx/resourcegroups/barfoo/providers/Microsoft.ManagedIdentity/userAssignedIdentities/foobar
  clientID: df398181-f42f-41b4-b791-b1d4572be315
```

Syncs to AzureIdentityBinding (to allow recreation eg in development environments)
```yaml
apiVersion: aadpodidentity.k8s.io/v1
kind: AzureIdentityBinding
metadata:
  labels:
    # used for sync AzureIdentity (eg. if recreated) to AzureIdentityBinding
    # if --azureidentitybinding.sync is used
    azure.k8s.io/msi-resourcegroup: barfoo
    azure.k8s.io/msi-resourcename: foobar
    azure.k8s.io/msi-subscription: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  name: foobar
  namespace: test123
spec:
  AzureIdentity: foobar-df398181-f42f-41b4-b791-b1d4572be315
  Selector: your-selector
```

Configuration
-------------

```
Usage:
  azure-msi-operator [OPTIONS]

Application Options:
  -v, --verbose                                Verbose mode [$VERBOSE]
      --sync.interval=                         Sync interval (time.duration) (default: 1h) [$SYNC_INTERVAL]
      --azure.subscription=                    Azure subscription ID [$AZURE_SUBSCRIPTION_ID]
      --kubeconfig=                            Kuberentes config path (should be empty if in-cluster) [$KUBECONFIG]
      --kubernetes.label.format=               Kubernetes label format (sprintf, if empty, labels are not set) (default: azure.k8s.io/%s) [$KUBERNETES_LABEL_FORMAT]
      --msi.namespaced                         Set aadpodidentity.k8s.io/Behavior=namespaced annotation [$MSI_NAMESPACED]
      --msi.template.namespace=                Golang template for Kubernetes namespace (default: {{index .Tags "k8snamespace"}}) [$MSI_TEMPLATE_NAMESPACE]
      --msi.template.resourcename=             Golang template for Kubernetes resource name (default: {{ .Name }}-{{ .ClientId }}) [$MSI_TEMPLATE_RESOURCENAME]
      --azureidentity.scheme.group=            AzureIdentity scheme group name (default: aadpodidentity.k8s.io) [$AZUREIDENTITY_SCHEME_GROUP]
      --azureidentity.scheme.version=          AzureIdentity scheme version (default: v1) [$AZUREIDENTITY_SCHEME_VERSION]
      --azureidentity.scheme.resource=         AzureIdentity scheme resource name (singular) (default: AzureIdentity) [$AZUREIDENTITY_SCHEME_RESOURCE]
      --azureidentity.scheme.resources=        AzureIdentity scheme resources name (pural) (default: azureidentities) [$AZUREIDENTITY_SCHEME_RESOURCES]
      --azureidentitybinding.scheme.group=     AzureIdentityBinding scheme group name (default: aadpodidentity.k8s.io) [$AZUREIDENTITYBINDING_SCHEME_GROUP]
      --azureidentitybinding.scheme.version=   AzureIdentityBinding scheme version (default: v1) [$AZUREIDENTITYBINDING_SCHEME_VERSION]
      --azureidentitybinding.scheme.resource=  AzureIdentityBinding scheme resource name (singular) (default: AzureIdentityBinding) [$AZUREIDENTITYBINDING_SCHEME_RESOURCE]
      --azureidentitybinding.scheme.resources= AzureIdentityBinding scheme resources name (pural) (default: azureidentitybindings) [$AZUREIDENTITYBINDING_SCHEME_RESOURCES]
      --azureidentitybinding.sync              Sync AzureIdentity to AzureIdentityBinding using lookup label [$AZUREIDENTITYBINDING_SYNC]
      --bind=                                  Server address (default: :8080) [$SERVER_BIND]

Help Options:
  -h, --help                                   Show this help message
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
