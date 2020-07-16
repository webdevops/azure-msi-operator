Operator for Azure Managed Service Identity in Kubernetes
=========================================================

[![license](https://img.shields.io/github/license/webdevops/azure-msi-operator.svg)](https://github.com/webdevops/azure-msi-operator/blob/master/LICENSE)
[![Docker](https://img.shields.io/docker/cloud/automated/webdevops/azure-msi-operator)](https://hub.docker.com/r/webdevops/azure-msi-operator/)
[![Docker Build Status](https://img.shields.io/docker/cloud/build/webdevops/azure-msi-operator)](https://hub.docker.com/r/webdevops/azure-msi-operator/)

Operator for Azure Managed Service Identity (MSI) in Kubernetes, requires [Azure aad-pod-identity service](https://github.com/Azure/aad-pod-identity)
Finds and security creates `AzureIdentity` resources in Kubernetes automatically when found in Azure:

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
    azure.k8s.io/msi-resourcegroup: barfoo
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
    azure.k8s.io/msi-resourcename: foobar
    azure.k8s.io/msi-resourcegroup: barfoo
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
      --debug                                debug mode [$DEBUG]
  -v, --verbose                              verbose mode [$VERBOSE]
      --log.json                             Switch log output to json format [$LOG_JSON]
      --sync.interval=                       Sync interval (time.duration) (default: 1h) [$SYNC_INTERVAL]
      --sync.watch                           Sync using namespace watch [$SYNC_WATCH]
      --sync.locktime=                       Lock time until next sync (time.duration) (default: 5m) [$SYNC_LOCKTIME]
      --azure.subscription=                  Azure subscription ID [$AZURE_SUBSCRIPTION_ID]
      --kubeconfig=                          Kuberentes config path (should be empty if in-cluster) [$KUBECONFIG]
      --kubernetes.label.format=             Kubernetes label format (sprintf, if empty, labels are not set) (default: azure.k8s.io/%s) [$KUBERNETES_LABEL_FORMAT]
      --azureidentity.namespaced             Set aadpodidentity.k8s.io/Behavior=namespaced annotation for AzureIdenity resources [$AZUREIDENTITY_NAMESPACED]
      --azureidentity.template.namespace=    Golang template for Kubernetes namespace (default: {{index .Tags "k8snamespace"}}) [$AZUREIDENTITY_TEMPLATE_NAMESPACE]
      --azureidentity.template.resourcename= Golang template for Kubernetes resource name (default: {{ .Name }}-{{ .ClientId }}) [$AZUREIDENTITY_TEMPLATE_RESOURCENAME]
      --azureidentitybinding.sync            Sync AzureIdentity to AzureIdentityBinding using lookup label [$AZUREIDENTITYBINDING_SYNC]
      --bind=                                Server address (default: :8080) [$SERVER_BIND]

Help Options:
  -h, --help                                 Show this help message
```

for Azure API authentication (using ENV vars) see https://github.com/Azure/azure-sdk-for-go#authentication


Templates
---------

[golang templates](https://golang.org/pkg/text/template/) are used to offer flexible customization for 
namespace (`--azureidentity.template.namespace`) and resourcename (`--azureidentity.template.resourcename`) 
detection/creation, following information are available:
```
    Id               string
    Name             string
    Location         string
    ResourceGroup    string
    SubscriptionId   string
    ClientId         string
    TenantId         string
    PrincipalID      string
    Tags             map[string]string
    Type             string
```

Examples :
```yaml
  env:
    # Use Azure ResourceName as AzureIdentity name (without ClientID)
    - name: AZUREIDENTITY_TEMPLATE_RESOURCENAME
      value "{{ .Name }}"

    # Use different Tag name for Namespace
    - name: AZUREIDENTITY_TEMPLATE_RESOURCENAME
      value: '{{index .Tags "namespace"}}'
```

Metrics
-------

| Metric                                         | Type         | Description                                                                           |
|------------------------------------------------|--------------|---------------------------------------------------------------------------------------|
| `azuremsi_sync_time`                           | Gauge        | Time (unix timestamp) of last sync run per Azure Subscription                         |
| `azuremsi_sync_duration`                       | Gauge        | Duration of last sync per Azure Subscription                                          |
| `azuremsi_sync_resources_errors`               | Counter      | Number of errors while syncing                                                        |
| `azuremsi_sync_resources_success`              | Counter      | Number of successfull syncs                                                           |
