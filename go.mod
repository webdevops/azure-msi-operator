module github.com/webdevops/azure-msi-operator

go 1.15

require (
	github.com/Azure/azure-sdk-for-go v44.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.0
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.1
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/jessevdk/go-flags v1.4.0
	github.com/operator-framework/operator-sdk v0.8.2
	github.com/prometheus/client_golang v1.0.0
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/sirupsen/logrus v1.4.2
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	sigs.k8s.io/controller-runtime v0.6.2 // indirect
)
