package main

import (
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	Author = "webdevops.io"
)

var (
	argparser *flags.Parser
	Verbose   bool
	Logger    *DaemonLogger

	// Git version information
	gitCommit = "<unknown>"
	gitTag    = "<unknown>"
)

var opts struct {
	// general settings
	Verbose []bool `long:"verbose" short:"v" env:"VERBOSE"      description:"Verbose mode"`

	// Sync settings
	SyncInterval time.Duration `long:"sync.interval" env:"SYNC_INTERVAL"  description:"Sync interval (time.duration)"  default:"1h"`

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

func main() {
	initArgparser()

	// set verbosity
	Verbose = len(opts.Verbose) >= 1

	Logger = NewLogger(log.Lshortfile, Verbose)
	defer Logger.Close()

	// set verbosity
	Verbose = len(opts.Verbose) >= 1

	Logger.Infof("starting Azure Managed Service Identity Operator v%s (%s; by %v)", gitTag, gitCommit, Author)

	operator := MsiOperator{}
	operator.Init()
	operator.Start(opts.SyncInterval)

	Logger.Infof("starting http server on %s", opts.ServerBind)
	startHttpServer()
}

// init argparser and parse/validate arguments
func initArgparser() {
	argparser = flags.NewParser(&opts, flags.Default)
	_, err := argparser.Parse()

	// check if there is an parse error
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			fmt.Println(err)
			fmt.Println()
			argparser.WriteHelp(os.Stdout)
			os.Exit(1)
		}
	}
}

// start and handle prometheus handler
func startHttpServer() {
	// healthz
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, "Ok"); err != nil {
			Logger.Error(err)
		}
	})

	// prom metrics
	http.Handle("/metrics", promhttp.Handler())

	Logger.Fatal(http.ListenAndServe(opts.ServerBind, nil))
}
