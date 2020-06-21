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
	KubernetesConfig string `long:"kubeconfig" env:"KUBECONFIG"  description:"Kuberentes config path (should be empty if in-cluster)"`

	// Msi settings
	MsiSchemeGroup          string `long:"msi.scheme.group"           env:"MSI_SCHEME_GROUP"           description:"MSI scheme group name" default:"aadpodidentity.k8s.io"`
	MsiSchemeVersion        string `long:"msi.scheme.version"         env:"MSI_SCHEME_VERSION"         description:"MSI scheme version" default:"v1"`
	MsiSchemeResource       string `long:"msi.scheme.resource"        env:"MSI_SCHEME_RESOURCE"        description:"MSI scheme resource name (singular)" default:"AzureIdentity"`
	MsiSchemeResources      string `long:"msi.scheme.resources"       env:"MSI_SCHEME_RESOURCES"       description:"MSI scheme resources name (pural)" default:"azureidentities"`
	MsiNamespaced           bool   `long:"msi.namespaced"             env:"MSI_NAMESPACED"             description:"Set aadpodidentity.k8s.io/Behavior=namespaced annotation"`
	MsiTagNamespace         string `long:"msi.tag.namespace"          env:"MSI_TAG_NAMESPACE"          description:"Name of Kubernetes namespace" default:"k8snamespace"`
	MsiTemplateResourceName string `long:"msi.template.resourcename"  env:"MSI_TEMPLATE_RESOURCENAME"  description:"Golang template for Kubernetes resource name" default:"{{ .Name }}-{{ .ClientId }}"`

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

	Logger.Infof("Starting Azure Managed Service Identity Operator v%s (%s; by %v)", gitTag, gitCommit, Author)

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
	http.Handle("/metrics", promhttp.Handler())
	Logger.Fatal(http.ListenAndServe(opts.ServerBind, nil))
}
