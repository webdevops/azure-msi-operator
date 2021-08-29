package config

import (
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"time"
)

type Opts struct {
	// logger
	Logger struct {
		Debug   bool `           long:"debug"        env:"DEBUG"    description:"debug mode"`
		Verbose bool `short:"v"  long:"verbose"      env:"VERBOSE"  description:"verbose mode"`
		LogJson bool `           long:"log.json"     env:"LOG_JSON" description:"Switch log output to json format"`
	}

	// instance
	Instance struct {
		Nodename  *string `long:"instance.nodename"    env:"INSTANCE_NODENAME"    description:"Name of node where autopilot is running"`
		Namespace *string `long:"instance.namespace"   env:"INSTANCE_NAMESPACE"   description:"Name of namespace where autopilot is running"`
		Pod       *string `long:"instance.pod"         env:"INSTANCE_POD"         description:"Name of pod where autopilot is running"`
	}

	// lease
	Lease struct {
		Enabled bool   `long:"lease.enable"  env:"LEASE_ENABLE"  description:"Enable lease (leader election; enabled by default in docker images)"`
		Name    string `long:"lease.name"    env:"LEASE_NAME"    description:"Name of lease lock"     default:"azure-msi-operator-leader"`
	}

	// Sync settings
	Sync struct {
		Interval time.Duration `long:"sync.interval" env:"SYNC_INTERVAL"  description:"Sync interval (time.duration)"  default:"1h"`
		Watch    bool          `long:"sync.watch"    env:"SYNC_WATCH"     description:"Sync using namespace watch"`
		LockTime time.Duration `long:"sync.locktime" env:"SYNC_LOCKTIME"  description:"Lock time until next sync (time.duration)" default:"5m"`
	}

	// azure settings
	Azure struct {
		Environment  string   `long:"azure.environment"   env:"AZURE_ENVIRONMENT"                    description:"Azure environment name" default:"AZUREPUBLICCLOUD"`
		Subscription []string `long:"azure.subscription"  env:"AZURE_SUBSCRIPTION_ID" env-delim:" "  description:"Azure subscription ID"`
	}

	// kubernetes settings
	Kubernetes struct {
		Config          string   `long:"kubeconfig" env:"KUBECONFIG"                                                 description:"Kuberentes config path (should be empty if in-cluster)"`
		LabelFormat     string   `long:"kubernetes.label.format" env:"KUBERNETES_LABEL_FORMAT"                       description:"Kubernetes label format (sprintf, if empty, labels are not set)" default:"msi.azure.k8s.io/%s"`
		NamespaceIgnore []string `long:"kubernetes.namespace.ignore" env:"KUBERNETES_NAMESPACE_IGNORE" env-delim:" " description:"Do not not maintain these namespaces" default:"kube-system" default:"kube-public" default:"default" default:"gatekeeper-system" default:"istio-system"` //nolint:golint,staticcheck
	}

	// AzureIdentity
	AzureIdentity struct {
		// Msi settings
		Namespaced           bool   `long:"azureidentity.namespaced"             env:"AZUREIDENTITY_NAMESPACED"             description:"Set aadpodidentity.k8s.io/Behavior=namespaced annotation for AzureIdenity resources"`
		TemplateNamespace    string `long:"azureidentity.template.namespace"     env:"AZUREIDENTITY_TEMPLATE_NAMESPACE"     description:"Golang template for Kubernetes namespace" default:"{{index .Tags \"k8snamespace\"}}"`
		TemplateResourceName string `long:"azureidentity.template.resourcename"  env:"AZUREIDENTITY_TEMPLATE_RESOURCENAME"  description:"Golang template for Kubernetes resource name" default:"{{ .Name }}-{{ .ClientId }}"`

		Binding struct {
			Sync bool `long:"azureidentity.binding.sync"  env:"AZUREIDENTITY_BINDING_SYNC"  description:"Sync AzureIdentity to AzureIdentityBinding using lookup label"`
		}

		Expiry struct {
			Enable     bool          `long:"azureidentity.expiry"             env:"AZUREIDENTITY_EXPIRY"              description:"Enable setting of expiry for removal of old AzureIdentity resources (use with hjacobs/kube-janitor)"`
			Annotation string        `long:"azureidentity.expiry.annotation"  env:"AZUREIDENTITY_EXPIRY_ANNOTATION"   description:"Name of expiry annotation" default:"janitor/expires"`
			Duration   time.Duration `long:"azureidentity.expiry.duration"    env:"AZUREIDENTITY_EXPIRY_DURATION"     description:"Duration of expiry value (time.Duration)" default:"2190h"`
			TimeFormat string        `long:"azureidentity.expiry.timeformat"  env:"AZUREIDENTITY_EXPIRY_TIMEFORMAT"   description:"Format of absolute time" default:"2006-01-02"`
		}
	}

	// server settings
	ServerBind string `long:"bind" env:"SERVER_BIND"  description:"Server address"  default:":8080"`
}

func (o *Opts) GetJson() []byte {
	jsonBytes, err := json.Marshal(o)
	if err != nil {
		log.Panic(err)
	}
	return jsonBytes
}
