module github.com/webdevops/azure-msi-operator

go 1.14

require (
	cloud.google.com/go v0.49.0 // indirect
	github.com/Azure/azure-sdk-for-go v36.1.0+incompatible
	github.com/Azure/go-autorest/autorest v0.9.3
	github.com/Azure/go-autorest/autorest/azure/auth v0.4.2
	github.com/Azure/go-autorest/autorest/to v0.3.1-0.20191028180845-3492b2aff503 // indirect
	github.com/google/go-cmp v0.4.0 // indirect
	github.com/google/logger v1.1.0
	github.com/gophercloud/gophercloud v0.6.0 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/jessevdk/go-flags v1.4.0
	github.com/prometheus/client_golang v1.7.0
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/stretchr/testify v1.5.1 // indirect
	golang.org/x/crypto v0.0.0-20200414173820-0848c9571904 // indirect
	golang.org/x/net v0.0.0-20200301022130-244492dfa37a // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v12.0.0+incompatible
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.18.2 // Required by prometheus-operator
)
