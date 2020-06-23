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

const (
	// AzureIdentity
	K8sSchemeAzureIdentityGroup            = "aadpodidentity.k8s.io"
	K8sSchemeAzureIdentityVersion          = "v1"
	K8sSchemeAzureIdentityResourceSingular = "AzureIdentity"
	K8sSchemeAzureIdentityResourcePlural   = "azureidentities"

	// AzureIdentityBinding
	K8sSchemeAzureIdentityBindingGroup            = "aadpodidentity.k8s.io"
	K8sSchemeAzureIdentityBindingVersion          = "v1"
	K8sSchemeAzureIdentityBindingResourceSingular = "AzureIdentityBinding"
	K8sSchemeAzureIdentityBindingResourcePlural   = "azureidentitybindings"
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
			msiResourcs       *prometheus.GaugeVec
			msiResourcsSuccess *prometheus.CounterVec
			msiResourceErrors *prometheus.CounterVec
			lastSync          *prometheus.GaugeVec
			duration          *prometheus.GaugeVec
		}

		msi struct {
			resourceNameTemplate *template.Template
			namespaceTemplate    *template.Template
		}
	}

	MsiResourceInfo struct {
		Msi                    *msi.Identity
		AzureResourceName      *string
		AzureResourceGroup     *string
		AzureSubscriptionId    *string
		KubernetesResourceName *string
		KubernetesNamespace    *string
	}
)

func (m *MsiOperator) Init() {
	m.initAzure()
	m.initKubernetes()
	m.initPrometheus()

	if t, err := template.New("msiResourceName").Parse(opts.AzureIdentityTemplateResourceName); err == nil {
		m.msi.resourceNameTemplate = t
	} else {
		panic(err)
	}

	if t, err := template.New("msiNamespace").Parse(opts.AzureIdentityTemplateNamespace); err == nil {
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
	m.prometheus.msiResourcsSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "azuremsi_sync_resources_success",
			Help: "Azure MSI operator successfull resource syncs",
		},
		[]string{"subscription", "resource"},
	)
	prometheus.MustRegister(m.prometheus.msiResourcsSuccess)

	m.prometheus.msiResourceErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "azuremsi_sync_resources_errors",
			Help: "Azure MSI operator failed resource syncs",
		},
		[]string{"subscription", "resource"},
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
			overallStartTime := time.Now()

			for _, subscription := range m.azure.subscriptionList {
				subscriptionStartTime := time.Now()
				Logger.Infof("using Azure Subscription \"%s\" (%s)", *subscription.DisplayName, *subscription.SubscriptionID)
				err := m.upsertSubscription(&subscription)
				if err != nil {
					Logger.Error(err)
				}

				subscriptionSyncDuration := time.Now().Sub(subscriptionStartTime)
				m.prometheus.duration.WithLabelValues(*subscription.SubscriptionID).Set(subscriptionSyncDuration.Seconds())
				m.prometheus.lastSync.WithLabelValues(*subscription.SubscriptionID).SetToCurrentTime()
			}

			overallDuration := time.Now().Sub(overallStartTime)
			Logger.Infof("finished after %s, waiting %s for next sync", overallDuration.String(), syncInterval.String())
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

	gvr := schema.GroupVersionResource{Group: K8sSchemeAzureIdentityGroup, Version: K8sSchemeAzureIdentityVersion, Resource: K8sSchemeAzureIdentityResourcePlural}
	for _, msi := range msiList {
		msiInfo, err := m.generateMsiKubernetesResourceInfo(msi)
		if err != nil {
			Logger.Error(err)
			continue
		}

		// check if namespace/resource was found
		if msiInfo.KubernetesNamespace == nil {
			Logger.Verbosef("unable to generate Kubernetes namespace name for Azure MSI %v", *msi.ID)
			continue
		}

		if msiInfo.KubernetesResourceName == nil {
			Logger.Verbosef("unable to generate Kubernetes resource name for Azure MSI %v", *msi.ID)
			continue
		}

		k8sNamespace := *msiInfo.KubernetesNamespace
		k8sResourceName := *msiInfo.KubernetesResourceName

		k8sPodIdentity, _ := m.kubernetes.client.Resource(gvr).Namespace(k8sNamespace).Get(ctx, k8sResourceName, metav1.GetOptions{})
		if k8sPodIdentity != nil {
			// update
			Logger.Infof("updating AzureIdentity %v/%v for %v", k8sNamespace, k8sResourceName, *msi.ID)

			if err := m.applyMsiToK8sObject(msi, k8sPodIdentity); err != nil {
				Logger.Error(err)
				continue
			}

			_, err := m.kubernetes.client.Resource(gvr).Namespace(k8sNamespace).Update(ctx, k8sPodIdentity, metav1.UpdateOptions{})
			if err != nil {
				Logger.Error(err)
				m.prometheus.msiResourceErrors.WithLabelValues(*subscription.SubscriptionID, K8sSchemeAzureIdentityResourceSingular).Inc()
			} else {
				m.prometheus.msiResourcsSuccess.WithLabelValues(*subscription.SubscriptionID, K8sSchemeAzureIdentityResourceSingular).Inc()
			}
		} else {
			// create
			Logger.Infof("creating AzureIdentity %v/%v for %v", k8sNamespace, k8sResourceName, *msi.ID)

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
				m.prometheus.msiResourceErrors.WithLabelValues(*subscription.SubscriptionID, K8sSchemeAzureIdentityResourceSingular).Inc()
			} else {
				m.prometheus.msiResourcsSuccess.WithLabelValues(*subscription.SubscriptionID, K8sSchemeAzureIdentityResourceSingular).Inc()
			}
		}

		if opts.AzureIdentityBindingSync {
			err := m.syncAzureIdentityToAzureIdentityBinding(msiInfo)
			if err != nil {
				Logger.Error(err)
			}
		}
	}

	return nil
}

func (m *MsiOperator) syncAzureIdentityToAzureIdentityBinding(msiInfo MsiResourceInfo) error {
	ctx := context.Background()
	gvr := schema.GroupVersionResource{Group: K8sSchemeAzureIdentityBindingGroup, Version: K8sSchemeAzureIdentityBindingVersion, Resource: K8sSchemeAzureIdentityBindingResourcePlural}

	labelSubscription := fmt.Sprintf(opts.KubernetesLabelFormat, "msi-subscription")
	labelResourceGroup := fmt.Sprintf(opts.KubernetesLabelFormat, "msi-resourcegroup")
	labelName := fmt.Sprintf(opts.KubernetesLabelFormat, "msi-resourcename")

	listOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf(
			"%s=%s,%s=%s,%s=%s",
			labelSubscription, *msiInfo.AzureSubscriptionId,
			labelResourceGroup, *msiInfo.AzureResourceGroup,
			labelName, *msiInfo.AzureResourceName,
		),
	}
	list, _ := m.kubernetes.client.Resource(gvr).Namespace(*msiInfo.KubernetesNamespace).List(ctx, listOpts)

	if list != nil {
		for _, azureIdentityBinding := range list.Items {
			if err := unstructured.SetNestedField(azureIdentityBinding.Object, *msiInfo.KubernetesResourceName, "spec", "AzureIdentity"); err != nil {
				return fmt.Errorf("failed to set object kind value: %v", err)
			}

			_, err := m.kubernetes.client.Resource(gvr).Namespace(*msiInfo.KubernetesNamespace).Update(ctx, &azureIdentityBinding, metav1.UpdateOptions{})
			if err != nil {
				Logger.Error(err)
				m.prometheus.msiResourceErrors.WithLabelValues(*msiInfo.AzureSubscriptionId, K8sSchemeAzureIdentityBindingResourceSingular).Inc()
			} else {
				m.prometheus.msiResourcsSuccess.WithLabelValues(*msiInfo.AzureSubscriptionId, K8sSchemeAzureIdentityBindingResourceSingular).Inc()
			}
		}
	}

	return nil
}

func (m *MsiOperator) generateMsiKubernetesResourceInfo(msi *msi.Identity) (msiInfo MsiResourceInfo, err error) {
	msiInfo = MsiResourceInfo{}

	resourceInfo, parseErr := azure.ParseResourceID(*msi.ID)
	if parseErr != nil {
		err = parseErr
		return
	}

	ResourceTags := map[string]string{}
	for tagName, tagValue := range msi.Tags {
		if tagValue != nil {
			ResourceTags[tagName] = *tagValue
		}
	}

	templateData := struct {
		Id             string
		Name           string
		Location       string
		ResourceGroup  string
		SubscriptionId string
		ClientId       string
		TenantId       string
		PrincipalID    string
		Tags           map[string]string
		Type           string
	}{
		Id:             *msi.ID,
		Name:           *msi.Name,
		Location:       *msi.Location,
		ResourceGroup:  resourceInfo.ResourceGroup,
		SubscriptionId: resourceInfo.SubscriptionID,
		ClientId:       msi.ClientID.String(),
		TenantId:       msi.TenantID.String(),
		PrincipalID:    msi.PrincipalID.String(),
		Tags:           ResourceTags,
		Type:           *msi.Type,
	}

	msiInfo.Msi = msi
	msiInfo.AzureResourceName = &resourceInfo.ResourceName
	msiInfo.AzureResourceGroup = &resourceInfo.ResourceGroup
	msiInfo.AzureSubscriptionId = &resourceInfo.SubscriptionID

	resNameBuf := &bytes.Buffer{}
	if err := m.msi.resourceNameTemplate.Execute(resNameBuf, templateData); err != nil {
		panic(err)
	}
	if val := resNameBuf.String(); val != "" {
		msiInfo.KubernetesResourceName = &val
	}

	namespaceBuf := &bytes.Buffer{}
	if err := m.msi.namespaceTemplate.Execute(namespaceBuf, templateData); err != nil {
		panic(err)
	}
	if val := namespaceBuf.String(); val != "" {
		msiInfo.KubernetesNamespace = &val
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
	resourceApiVersion := fmt.Sprintf("%s/%s", K8sSchemeAzureIdentityBindingGroup, K8sSchemeAzureIdentityBindingVersion)
	if err := unstructured.SetNestedField(k8sResource.Object, resourceApiVersion, "apiVersion"); err != nil {
		return fmt.Errorf("failed to set object apiversion value: %v", err)
	}

	if err := unstructured.SetNestedField(k8sResource.Object, K8sSchemeAzureIdentityResourceSingular, "kind"); err != nil {
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
	if opts.AzureIdentityNamespaced {
		if err := unstructured.SetNestedField(k8sResource.Object, "namespaced", "metadata", "annotations", "aadpodidentity.k8s.io/Behavior"); err != nil {
			return fmt.Errorf("failed to set metadata.annotations[aadpodidentity.k8s.io/Behavior] value: %v", err)
		}
	} else {
		unstructured.RemoveNestedField(k8sResource.Object, "metadata", "annotations", "aadpodidentity.k8s.io/Behavior")
	}

	// labels
	if opts.KubernetesLabelFormat != "" {
		labelName := fmt.Sprintf(opts.KubernetesLabelFormat, "msi-subscription")
		if err := unstructured.SetNestedField(k8sResource.Object, resourceInfo.SubscriptionID, "metadata", "labels", labelName); err != nil {
			return fmt.Errorf("failed to set metadata.labels[%v] value: %v", labelName, err)
		}

		labelName = fmt.Sprintf(opts.KubernetesLabelFormat, "msi-resourcerroup")
		if err := unstructured.SetNestedField(k8sResource.Object, resourceInfo.ResourceGroup, "metadata", "labels", labelName); err != nil {
			return fmt.Errorf("failed to set metadata.labels[%v] value: %v", labelName, err)
		}

		labelName = fmt.Sprintf(opts.KubernetesLabelFormat, "msi-resourcename")
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
