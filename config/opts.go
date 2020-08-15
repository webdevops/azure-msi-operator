package config

import "time"

type Opts struct {
	// logger
	Logger struct {
		Debug   bool `           long:"debug"        env:"DEBUG"    description:"debug mode"`
		Verbose bool `short:"v"  long:"verbose"      env:"VERBOSE"  description:"verbose mode"`
		LogJson bool `           long:"log.json"     env:"LOG_JSON" description:"Switch log output to json format"`
	}

	// instance
	Instance struct {
		Nodename  *string `long:"instance.nodename"    env:"INSTANCE_NODENAME"   description:"Name of node where autopilot is running"`
		Namespace *string `long:"instance.namespace"   env:"INSTANCE_NAMESPACE"   description:"Name of namespace where autopilot is running"`
		Pod       *string `long:"instance.pod"         env:"INSTANCE_POD"         description:"Name of pod where autopilot is running"`
	}

	K8s struct {
		NodeLabelSelector string `long:"kube.node.labelselector"     env:"KUBE_NODE_LABELSELECTOR"     description:"Node Label selector which nodes should be checked"        default:""`
	}

	// lease
	Lease struct {
		Enabled bool   `long:"lease.enable"  env:"LEASE_ENABLE"  description:"Enable lease (leader election; enabled by default in docker images)"`
		Name    string `long:"lease.name"    env:"LEASE_NAME"    description:"Name of lease lock"     default:"azure-msi-operator-leader"`
	}

	// Sync settings
	SyncInterval time.Duration `long:"sync.interval" env:"SYNC_INTERVAL"  description:"Sync interval (time.duration)"  default:"1h"`
	SyncWatch    bool          `long:"sync.watch"    env:"SYNC_WATCH"     description:"Sync using namespace watch"`
	SyncLockTime time.Duration `long:"sync.locktime" env:"SYNC_LOCKTIME"  description:"Lock time until next sync (time.duration)" default:"5m"`

	// azure settings
	AzureSubscription []string `long:"azure.subscription" env:"AZURE_SUBSCRIPTION_ID" env-delim:" "  description:"Azure subscription ID"`

	// kubernetes settings
	KubernetesConfig      string `long:"kubeconfig" env:"KUBECONFIG"  description:"Kuberentes config path (should be empty if in-cluster)"`
	KubernetesLabelFormat string `long:"kubernetes.label.format" env:"KUBERNETES_LABEL_FORMAT"  description:"Kubernetes label format (sprintf, if empty, labels are not set)" default:"azure.k8s.io/%s"`

	// Msi settings
	AzureIdentityNamespaced           bool   `long:"azureidentity.namespaced"             env:"AZUREIDENTITY_NAMESPACED"             description:"Set aadpodidentity.k8s.io/Behavior=namespaced annotation for AzureIdenity resources"`
	AzureIdentityTemplateNamespace    string `long:"azureidentity.template.namespace"     env:"AZUREIDENTITY_TEMPLATE_NAMESPACE"     description:"Golang template for Kubernetes namespace" default:"{{index .Tags \"k8snamespace\"}}"`
	AzureIdentityTemplateResourceName string `long:"azureidentity.template.resourcename"  env:"AZUREIDENTITY_TEMPLATE_RESOURCENAME"  description:"Golang template for Kubernetes resource name" default:"{{ .Name }}-{{ .ClientId }}"`

	// AzureIdentityBinding
	AzureIdentityBindingSync bool `long:"azureidentitybinding.sync"  env:"AZUREIDENTITYBINDING_SYNC"  description:"Sync AzureIdentity to AzureIdentityBinding using lookup label"`

	// server settings
	ServerBind string `long:"bind" env:"SERVER_BIND"  description:"Server address"  default:":8080"`
}
