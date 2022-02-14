# Operator for Azure Managed Service Identity in Kubernetes (for aad-pod-identity)

[![license](https://img.shields.io/github/license/webdevops/azure-msi-operator.svg)](https://github.com/webdevops/azure-msi-operator/blob/master/LICENSE)
[![DockerHub](https://img.shields.io/badge/DockerHub-webdevops%2Fazure--msi--operator-blue)](https://hub.docker.com/r/webdevops/azure-msi-operator/)
[![Quay.io](https://img.shields.io/badge/Quay.io-webdevops%2Fazure--msi--operator-blue)](https://quay.io/repository/webdevops/azure-msi-operator)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/azure-msi-operator)](https://artifacthub.io/packages/search?repo=azure-msi-operator)

Operator for Azure Managed Service Identity (MSI) in Kubernetes, requires [Azure aad-pod-identity service](https://github.com/Azure/aad-pod-identity)

Why using this app?
Because it can be a security issue if developers can create `AzureIdentity` resources and could take over
other teams Azure UserAssignedIdentity (MSI) resources.

This operator automates the process and detaches them from the developers responsibility.
It looks up the configured namespaces (default configuration) and syncs `AzureIdentity` resources into the specified
Kubernetes namespace. Then it checks `AzureIdentityBinding` resources for labels to
bind `AzureIdentity` and `AzureIdentityBinding` together in a secure way.

## Features

- automatically creates and maintains `AzureIdentity` resources in Kubernetes
- extracts Namespace from MSI tag resource (can be configured)
- automatically syncs `AzureIdentity` to `AzureIdentityBinding` using labels (simplifies deployments)
- allows to configure the name of `AzureIdentity` and namespace settings
- support expiry of `AzureIdentity` resources (use (hjacobs/kube-janitor)[https://codeberg.org/hjacobs/kube-janitor])
- leader election support (allows to run the operator multiple times with fast handover)
- supports `Namespace` creation and `AzureIdentityBinding` creating and modification watch in Kubernetes (allows fast and intelligent sync)
- exposes Prometheus metrics

## Usage

```
Usage:
  azure-msi-operator [OPTIONS]

Application Options:
      --debug                                debug mode [$DEBUG]
  -v, --verbose                              verbose mode [$VERBOSE]
      --log.json                             Switch log output to json format [$LOG_JSON]
      --instance.nodename=                   Name of node where autopilot is running [$INSTANCE_NODENAME]
      --instance.namespace=                  Name of namespace where autopilot is running [$INSTANCE_NAMESPACE]
      --instance.pod=                        Name of pod where autopilot is running [$INSTANCE_POD]
      --lease.enable                         Enable lease (leader election; enabled by default in docker images) [$LEASE_ENABLE]
      --lease.name=                          Name of lease lock (default: azure-msi-operator-leader) [$LEASE_NAME]
      --sync.interval=                       Sync interval (time.duration) (default: 1h) [$SYNC_INTERVAL]
      --sync.watch                           Sync using namespace watch [$SYNC_WATCH]
      --sync.locktime=                       Lock time until next sync (time.duration) (default: 5m) [$SYNC_LOCKTIME]
      --azure.environment=                   Azure environment name (default: AZUREPUBLICCLOUD) [$AZURE_ENVIRONMENT]
      --azure.subscription=                  Azure subscription ID [$AZURE_SUBSCRIPTION_ID]
      --kubeconfig=                          Kuberentes config path (should be empty if in-cluster) [$KUBECONFIG]
      --kubernetes.label.format=             Kubernetes label format (sprintf, if empty, labels are not set) (default: msi.azure.k8s.io/%s)
                                             [$KUBERNETES_LABEL_FORMAT]
      --kubernetes.namespace.ignore=         Do not not maintain these namespaces (default: kube-system, kube-public, default, gatekeeper-system,
                                             istio-system) [$KUBERNETES_NAMESPACE_IGNORE]
      --azureidentity.namespaced             Set aadpodidentity.k8s.io/Behavior=namespaced annotation for AzureIdenity resources
                                             [$AZUREIDENTITY_NAMESPACED]
      --azureidentity.template.namespace=    Golang template for Kubernetes namespace (default: {{index .Tags "k8snamespace"}})
                                             [$AZUREIDENTITY_TEMPLATE_NAMESPACE]
      --azureidentity.template.resourcename= Golang template for Kubernetes resource name (default: {{ .Name }}-{{ .ClientId }})
                                             [$AZUREIDENTITY_TEMPLATE_RESOURCENAME]
      --azureidentity.binding.sync           Sync AzureIdentity to AzureIdentityBinding using lookup label [$AZUREIDENTITY_BINDING_SYNC]
      --azureidentity.expiry                 Enable setting of expiry for removal of old AzureIdentity resources (use with hjacobs/kube-janitor)
                                             [$AZUREIDENTITY_EXPIRY]
      --azureidentity.expiry.annotation=     Name of expiry annotation (default: janitor/expires) [$AZUREIDENTITY_EXPIRY_ANNOTATION]
      --azureidentity.expiry.duration=       Duration of expiry value (time.Duration) (default: 2190h) [$AZUREIDENTITY_EXPIRY_DURATION]
      --azureidentity.expiry.timeformat=     Format of absolute time (default: 2006-01-02) [$AZUREIDENTITY_EXPIRY_TIMEFORMAT]
      --bind=                                Server address (default: :8080) [$SERVER_BIND]

Help Options:
  -h, --help                                 Show this help message
```

for Azure API authentication (using ENV vars) see https://docs.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication

## Example

Creates and maintains `AzureIdentity` resources in Kubernetes in an automated and safe way when found in Azure:

Example Azure MSI:
```yaml
ResourceName: foobar
ResourceGroup: barfoo
Subscription: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
ClientID: df398181-f42f-41b4-b791-b1d4572be315
Tags:
    # separate multiple namespaces with comma
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
    msi.azure.k8s.io/name: foobar
    msi.azure.k8s.io/resourcegroup: barfoo
    msi.azure.k8s.io/subscription: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  annotations:
      aadpodidentity.k8s.io/Behavior: namespaced #optional if namespaced mode is enabled
      janitor/expires: "2021-11-28" #optional if expiry is enabled
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
    msi.azure.k8s.io/name: foobar
    msi.azure.k8s.io/resourcegroup: barfoo
    msi.azure.k8s.io/subscription: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  name: foobar
  namespace: test123
spec:
  azureIdentity: foobar-df398181-f42f-41b4-b791-b1d4572be315
  selector: your-selector
```

## Templates

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

## Cleanup/expiry

This operator doesn't remove the `AzureIdentity` resources from your clusters to avoid any downtime because of eg. permissions
issues in ServiceDiscovery.
You can enable expiry annotations (`AZUREIDENTITY_EXPIRY`) and let them clean up with (hjacobs/kube-janitor)[https://codeberg.org/hjacobs/kube-janitor].

## Metrics

| Metric                                         | Type         | Description                                                                           |
|------------------------------------------------|--------------|---------------------------------------------------------------------------------------|
| `azuremsi_sync_time`                           | Gauge        | Time (unix timestamp) of last sync run per Azure Subscription                         |
| `azuremsi_sync_duration`                       | Gauge        | Duration of last sync per Azure Subscription                                          |
| `azuremsi_sync_resources_errors`               | Counter      | Number of errors while syncing                                                        |
| `azuremsi_sync_resources_success`              | Counter      | Number of successfull syncs                                                           |

## AzureTracing metrics

(with 22.2.0 and later)

Azuretracing metrics collects latency and latency from azure-sdk-for-go and creates metrics and is controllable using
environment variables (eg. setting buckets, disabling metrics or disable autoreset).

| Metric                                   | Description                                                                            |
|------------------------------------------|----------------------------------------------------------------------------------------|
| `azurerm_api_ratelimit`                  | Azure ratelimit metrics (only on /metrics, resets after query due to limited validity) |
| `azurerm_api_request_*`                  | Azure request count and latency as histogram                                           |

### Settings

| Environment variable                     | Example                            | Description                                                    |
|------------------------------------------|------------------------------------|----------------------------------------------------------------|
| `METRIC_AZURERM_API_REQUEST_BUCKETS`     | `1, 2.5, 5, 10, 30, 60, 90, 120`   | Sets buckets for `azurerm_api_request` histogram metric        |
| `METRIC_AZURERM_API_REQUEST_ENABLE`      | `false`                            | Enables/disables `azurerm_api_request_*` metric                |
| `METRIC_AZURERM_API_REQUEST_LABELS`      | `apiEndpoint, method, statusCode`  | Controls labels of `azurerm_api_request_*` metric              |
| `METRIC_AZURERM_API_RATELIMIT_ENABLE`    | `false`                            | Enables/disables `azurerm_api_ratelimit` metric                |
| `METRIC_AZURERM_API_RATELIMIT_AUTORESET` | `false`                            | Enables/disables `azurerm_api_ratelimit` autoreset after fetch |


| `azurerm_api_request` label | Status             | Description                                                                                              |
|-----------------------------|--------------------|----------------------------------------------------------------------------------------------------------|
| `apiEndpoint`               | enabled by default | hostname of endpoint (max 3 parts)                                                                       |
| `routingRegion`             | enabled by default | detected region for API call, either routing region from Azure Management API or Azure resource location |
| `subscriptionID`            | enabled by default | detected subscriptionID                                                                                  |
| `tenantID`                  | enabled by default | detected tenantID (extracted from jwt auth token)                                                        |
| `resourceProvider`          | enabled by default | detected Azure Management API provider                                                                   |
| `method`                    | enabled by default | HTTP method                                                                                              |
| `statusCode`                | enabled by default | HTTP status code                                                                                         |
