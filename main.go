package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"

	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/webdevops/go-common/prometheus/azuretracing"

	"github.com/webdevops/azure-msi-operator/config"
	"github.com/webdevops/azure-msi-operator/operator"
)

const (
	Author = "webdevops.io"
)

var (
	argparser *flags.Parser

	opts config.Opts

	// Git version information
	gitCommit = "<unknown>"
	gitTag    = "<unknown>"
)

func main() {
	initArgparser()

	log.Infof("starting azure-msi-operator v%s (%s; %s; by %v)", gitTag, gitCommit, runtime.Version(), Author)
	log.Info(string(opts.GetJson()))

	msiOperator := operator.MsiOperator{
		Conf:      opts,
		UserAgent: "azure-msi-operator/" + gitTag,
	}
	msiOperator.Init()
	msiOperator.Start(opts.Sync.Interval)

	log.Infof("starting http server on %s", opts.Server.Bind)
	startHttpServer()
}

// init argparser and parse/validate arguments
func initArgparser() {
	argparser = flags.NewParser(&opts, flags.Default)
	_, err := argparser.Parse()

	// check if there is an parse error
	if err != nil {
		var flagsErr *flags.Error
		if ok := errors.As(err, &flagsErr); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			fmt.Println(err)
			fmt.Println()
			argparser.WriteHelp(os.Stdout)
			os.Exit(1)
		}
	}

	// verbose level
	if opts.Logger.Verbose {
		log.SetLevel(log.DebugLevel)
	}

	// debug level
	if opts.Logger.Debug {
		log.SetReportCaller(true)
		log.SetLevel(log.TraceLevel)
	}

	// json log format
	if opts.Logger.LogJson {
		log.SetFormatter(&log.JSONFormatter{})
	}
}

// start and handle prometheus handler
func startHttpServer() {
	mux := http.NewServeMux()

	// healthz
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, "Ok"); err != nil {
			log.Error(err)
		}
	})

	// readyz
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, "Ok"); err != nil {
			log.Error(err)
		}
	})

	// prom metrics
	mux.Handle("/metrics", azuretracing.RegisterAzureMetricAutoClean(promhttp.Handler()))

	srv := &http.Server{
		Addr:         opts.Server.Bind,
		Handler:      mux,
		ReadTimeout:  opts.Server.ReadTimeout,
		WriteTimeout: opts.Server.WriteTimeout,
	}
	log.Fatal(srv.ListenAndServe())
}
