module github.com/webdevops/azure-msi-operator

go 1.15

require (
	github.com/Azure/azure-sdk-for-go v48.2.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.11
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.3
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.0 // indirect
	github.com/go-logr/logr v0.2.1 // indirect
	github.com/jessevdk/go-flags v1.4.0
	github.com/operator-framework/operator-lib v0.2.0
	github.com/prometheus/client_golang v1.8.0
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/sirupsen/logrus v1.7.0
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	k8s.io/apimachinery v0.19.4
	k8s.io/client-go v0.19.4
)
