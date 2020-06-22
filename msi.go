package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/subscriptions"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"text/template"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

type (
	MsiOperator struct {
		kubernetes struct {
			client dynamic.Interface
		}

		azure struct {
			environment      azure.Environment
			authorizer       autorest.Authorizer
			subscriptionList []subscriptions.Subscription
		}

		prometheus struct {
			msiResourceSynced *prometheus.CounterVec
			msiResourceErrors *prometheus.CounterVec
			lastSync          *prometheus.GaugeVec
			duration          *prometheus.GaugeVec
		}

		msi struct {
			resourceNameTemplate *template.Template
			namespaceTemplate *template.Template
		}
	}
)

func (m *MsiOperator) Init() {
	m.initAzure()
	m.initKubernetes()
	m.initPrometheus()

	if t, err := template.New("msiResourceName").Parse(opts.MsiTemplateResourceName); err == nil {
		m.msi.resourceNameTemplate = t
	} else {
		panic(err)
	}

	if t, err := template.New("msiNamespace").Parse(opts.MsiTemplateNamespace); err == nil {
		m.msi.namespaceTemplate = t
	} else {
		panic(err)
	}
}

func (m *MsiOperator) initAzure() {
	var err error
	ctx := context.Background()

	// setup azure authorizer
	m.azure.authorizer, err = auth.NewAuthorizerFromEnvironment()
	if err != nil {
		panic(err)
	}
	subscriptionsClient := subscriptions.NewClient()
	subscriptionsClient.Authorizer = m.azure.authorizer

	if len(opts.AzureSubscription) == 0 {
		// auto lookup subscriptions
		listResult, err := subscriptionsClient.List(ctx)
		if err != nil {
			panic(err)
		}
		m.azure.subscriptionList = listResult.Values()

		if len(m.azure.subscriptionList) == 0 {
			panic(errors.New("no Azure Subscriptions found via auto detection, does this ServicePrincipal have read permissions to the subcriptions?"))
		}
	} else {
		// fixed subscription list
		m.azure.subscriptionList = []subscriptions.Subscription{}
		for _, subId := range opts.AzureSubscription {
			result, err := subscriptionsClient.Get(ctx, subId)
			if err != nil {
				panic(err)
			}
			m.azure.subscriptionList = append(m.azure.subscriptionList, result)
		}
	}

	// try to get cloud name, defaults to public cloud name
	azureEnvName := azure.PublicCloud.Name
	if env := os.Getenv("AZURE_ENVIRONMENT"); env != "" {
		azureEnvName = env
	}

	m.azure.environment, err = azure.EnvironmentFromName(azureEnvName)
	if err != nil {
		panic(err)
	}
}

func (m *MsiOperator) initKubernetes() {
	// get kubeconfig
	kubeconf, err := clientcmd.BuildConfigFromFlags("", opts.KubernetesConfig)
	if err != nil {
		panic(err)
	}

	// create kubernetes client
	client, err := dynamic.NewForConfig(kubeconf)
	if err != nil {
		panic(err)
	}

	m.kubernetes.client = client
}

func (m *MsiOperator) initPrometheus() {
	m.prometheus.msiResourceSynced = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "azuremsi_sync_resources_success",
			Help: "Azure MSI operator successfull resource syncs",
		},
		[]string{"subscription"},
	)
	prometheus.MustRegister(m.prometheus.msiResourceSynced)

	m.prometheus.msiResourceErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "azuremsi_sync_resources_errors",
			Help: "Azure MSI operator failed resource syncs",
		},
		[]string{"subscription"},
	)
	prometheus.MustRegister(m.prometheus.msiResourceErrors)

	m.prometheus.duration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "azuremsi_sync_duration",
			Help: "Azure MSI operator sync duration",
		},
		[]string{"subscription"},
	)
	prometheus.MustRegister(m.prometheus.duration)

	m.prometheus.lastSync = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "azuremsi_sync_time",
			Help: "Azure MSI operator last sync time",
		},
		[]string{"subscription"},
	)
	prometheus.MustRegister(m.prometheus.lastSync)
}

func (m *MsiOperator) Start(syncInterval time.Duration) {
	go func() {
		for {
			Logger.Info("starting sync")

			for _, subscription := range m.azure.subscriptionList {
				startTime := time.Now()
				Logger.Infof("using Azure Subscription \"%s\" (%s)", *subscription.DisplayName, *subscription.SubscriptionID)
				err := m.upsertSubscription(&subscription)
				if err != nil {
					Logger.Error(err)
				}

				syncDuration := time.Now().Sub(startTime)
				m.prometheus.duration.WithLabelValues(*subscription.SubscriptionID).Set(syncDuration.Seconds())
				m.prometheus.lastSync.WithLabelValues(*subscription.SubscriptionID).SetToCurrentTime()
			}

			Logger.Infof("finished, waiting %v for next sync", syncInterval)
			time.Sleep(syncInterval)
		}
	}()
}

func (m *MsiOperator) upsertSubscription(subscription *subscriptions.Subscription) error {
	ctx := context.Background()

	msiList, err := m.getAzureMsiList(subscription)
	if err != nil {
		return err
	}

	gvr := schema.GroupVersionResource{Group: opts.MsiSchemeGroup, Version: opts.MsiSchemeVersion, Resource: opts.MsiSchemeResources}
	for _, msi := range msiList {
		msiNamespace, msiResourceName, err := m.generateMsiKubernetesResourceInfo(msi)
		if err != nil {
			Logger.Error(err)
			continue
		}

		// check if namespace/resource was found
		if msiNamespace == nil || msiResourceName == nil {
			Logger.Verbosef("unable to generate Kubernetes namespace or resource name for Azure MSI %v", *msi.ID)
			continue
		}

		k8sNamespace := *msiNamespace
		k8sResourceName := *msiResourceName

		k8sPodIdentity, _ := m.kubernetes.client.Resource(gvr).Namespace(k8sNamespace).Get(ctx, k8sResourceName, metav1.GetOptions{})
		if k8sPodIdentity != nil {
			// update
			Logger.Verbosef("updating AzureIdentity %v/%v for %v", k8sNamespace, k8sResourceName, *msi.ID)

			if err := m.applyMsiToK8sObject(msi, k8sPodIdentity); err != nil {
				Logger.Error(err)
				continue
			}

			_, err := m.kubernetes.client.Resource(gvr).Namespace(k8sNamespace).Update(ctx, k8sPodIdentity, metav1.UpdateOptions{})
			if err != nil {
				Logger.Error(err)
				m.prometheus.msiResourceErrors.WithLabelValues(*subscription.SubscriptionID).Inc()
			} else {
				m.prometheus.msiResourceSynced.WithLabelValues(*subscription.SubscriptionID).Inc()
			}
		} else {
			// create
			Logger.Verbosef("creating AzureIdentity %v/%v for %v", k8sNamespace, k8sResourceName, *msi.ID)

			// object
			k8sPodIdentity = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":        k8sResourceName,
						"annotations": map[string]interface{}{},
						"labels":      map[string]interface{}{},
					},
					"spec": map[string]interface{}{},
				},
			}

			if err := m.applyMsiToK8sObject(msi, k8sPodIdentity); err != nil {
				Logger.Error(err)
				continue
			}

			_, err := m.kubernetes.client.Resource(gvr).Namespace(k8sNamespace).Create(ctx, k8sPodIdentity, metav1.CreateOptions{})
			if err != nil {
				Logger.Error(err)
				m.prometheus.msiResourceErrors.WithLabelValues(*subscription.SubscriptionID).Inc()
			} else {
				m.prometheus.msiResourceSynced.WithLabelValues(*subscription.SubscriptionID).Inc()
			}
		}
	}

	return nil
}

func (m *MsiOperator) generateMsiKubernetesResourceInfo(msi *msi.Identity) (namespaceName, resourceName *string, err error) {
	resourceInfo, parseErr := azure.ParseResourceID(*msi.ID)
	if parseErr != nil {
		err = parseErr
		return
	}

	templateData := struct {
		Name             string
		Location         string
		ResourceGroup    string
		SubscriptionName string
		ClientId         string
		TenantId         string
		Tags             map[string]*string
		Type             string
	}{
		Name:             *msi.Name,
		Location:         *msi.Location,
		ResourceGroup:    resourceInfo.ResourceGroup,
		SubscriptionName: resourceInfo.SubscriptionID,
		ClientId:         msi.ClientID.String(),
		TenantId:         msi.TenantID.String(),
		Tags:             msi.Tags,
		Type:             string(msi.Type),
	}

	resNameBuf := &bytes.Buffer{}
	if err := m.msi.resourceNameTemplate.Execute(resNameBuf, templateData); err != nil {
		panic(err)
	}
	if val := resNameBuf.String(); val != "" {
		resourceName = &val
	}

	namespaceBuf := &bytes.Buffer{}
	if err := m.msi.namespaceTemplate.Execute(namespaceBuf, templateData); err != nil {
		panic(err)
	}
	if val := namespaceBuf.String(); val != "" {
		namespaceName = &val
	}

	return
}

func (m *MsiOperator) applyMsiToK8sObject(msi *msi.Identity, k8sResource *unstructured.Unstructured) error {
	msiResourceId := string(*msi.ID)
	msiClientId := string(msi.ClientID.String())

	resourceInfo, err := azure.ParseResourceID(*msi.ID)
	if err != nil {
		return err
	}

	// main
	resourceApiVersion := fmt.Sprintf("%s/%s", opts.MsiSchemeGroup, opts.MsiSchemeVersion)
	if err := unstructured.SetNestedField(k8sResource.Object, resourceApiVersion, "apiVersion"); err != nil {
		return fmt.Errorf("failed to set object apiversion value: %v", err)
	}

	resourceKind := opts.MsiSchemeResource
	if err := unstructured.SetNestedField(k8sResource.Object, resourceKind, "kind"); err != nil {
		return fmt.Errorf("failed to set object kind value: %v", err)
	}

	// settings
	if err := unstructured.SetNestedField(k8sResource.Object, "0", "spec", "type"); err != nil {
		return fmt.Errorf("failed to set spec.type value: %v", err)
	}

	if err := unstructured.SetNestedField(k8sResource.Object, msiResourceId, "spec", "resourceID"); err != nil {
		return fmt.Errorf("failed to set spec.resourceID value: %v", err)
	}

	if err := unstructured.SetNestedField(k8sResource.Object, msiClientId, "spec", "clientID"); err != nil {
		return fmt.Errorf("failed to set spec.clientID value: %v", err)
	}

	// annotations
	if opts.MsiNamespaced {
		if err := unstructured.SetNestedField(k8sResource.Object, "namespaced", "metadata", "annotations", "aadpodidentity.k8s.io/Behavior"); err != nil {
			return fmt.Errorf("failed to set metadata.annotations[aadpodidentity.k8s.io/Behavior] value: %v", err)
		}
	} else {
		unstructured.RemoveNestedField(k8sResource.Object, "metadata", "annotations", "aadpodidentity.k8s.io/Behavior")
	}

	// labels
	if opts.KubernetesLabelFormat != "" {
		labelName := fmt.Sprintf(opts.KubernetesLabelFormat, "subscription")
		if err := unstructured.SetNestedField(k8sResource.Object, resourceInfo.SubscriptionID, "metadata", "labels", labelName); err != nil {
			return fmt.Errorf("failed to set metadata.labels[%v] value: %v", labelName, err)
		}

		labelName = fmt.Sprintf(opts.KubernetesLabelFormat, "resourceGroup")
		if err := unstructured.SetNestedField(k8sResource.Object, resourceInfo.ResourceGroup, "metadata", "labels", labelName); err != nil {
			return fmt.Errorf("failed to set metadata.labels[%v] value: %v", labelName, err)
		}

		labelName = fmt.Sprintf(opts.KubernetesLabelFormat, "resourceName")
		if err := unstructured.SetNestedField(k8sResource.Object, resourceInfo.ResourceName, "metadata", "labels", labelName); err != nil {
			return fmt.Errorf("failed to set metadata.labels[%v] value: %v", labelName, err)
		}
	}


	return nil
}

func (m *MsiOperator) getAzureMsiList(subscription *subscriptions.Subscription) (ret []*msi.Identity, err error) {
	ctx := context.Background()

	client := msi.NewUserAssignedIdentitiesClient(*subscription.SubscriptionID)
	client.Authorizer = m.azure.authorizer

	list, azureErr := client.ListBySubscriptionComplete(ctx)
	if azureErr != nil {
		err = azureErr
		return
	}

	for list.NotDone() {
		result := list.Value()
		ret = append(ret, &result)
		if list.NextWithContext(ctx) != nil {
			break
		}
	}

	return
}
