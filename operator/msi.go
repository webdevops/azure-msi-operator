package operator

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
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/operator-framework/operator-lib/leader"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/webdevops/azure-msi-operator/config"
	"golang.org/x/sync/semaphore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
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
		Conf config.Opts
		ctx  context.Context
		lock *semaphore.Weighted

		kubernetes struct {
			client dynamic.Interface
		}

		azure struct {
			environment      azure.Environment
			authorizer       autorest.Authorizer
			subscriptionList []subscriptions.Subscription
		}

		prometheus struct {
			msiResourcs        *prometheus.GaugeVec
			msiResourcsSuccess *prometheus.CounterVec
			msiResourceErrors  *prometheus.CounterVec
			lastSync           *prometheus.GaugeVec
			duration           *prometheus.GaugeVec
		}

		msi struct {
			resourceNameTemplate *template.Template
			namespaceTemplate    *template.Template
		}
	}

	MsiResourceInfo struct {
		Msi                    *msi.Identity
		AzureResourceId        *string
		AzureResourceName      *string
		AzureResourceGroup     *string
		AzureSubscriptionId    *string
		KubernetesResourceName *string
		KubernetesNamespace    []string
	}
)

func (m *MsiOperator) Init() {
	m.ctx = context.Background()
	m.lock = semaphore.NewWeighted(1)

	m.initAzure()
	m.initKubernetes()
	m.initPrometheus()

	if t, err := template.New("msiResourceName").Parse(m.Conf.AzureMsi.TemplateResourceName); err == nil {
		m.msi.resourceNameTemplate = t
	} else {
		panic(err)
	}

	if t, err := template.New("msiNamespace").Parse(m.Conf.AzureMsi.TemplateNamespace); err == nil {
		m.msi.namespaceTemplate = t
	} else {
		panic(err)
	}
}

func (m *MsiOperator) initAzure() {
	var err error
	// setup azure authorizer
	m.azure.authorizer, err = auth.NewAuthorizerFromEnvironment()
	if err != nil {
		panic(err)
	}
	subscriptionsClient := subscriptions.NewClient()
	subscriptionsClient.Authorizer = m.azure.authorizer

	if len(m.Conf.Azure.Subscription) == 0 {
		// auto lookup subscriptions
		listResult, err := subscriptionsClient.List(m.ctx)
		if err != nil {
			panic(err)
		}
		m.azure.subscriptionList = listResult.Values()

		if len(m.azure.subscriptionList) == 0 {
			panic(errors.New("no Azure Subscriptions found via auto detection or ServicePrincipal doesn't have permission to read subscriptions"))
		}
	} else {
		// fixed subscription list
		m.azure.subscriptionList = []subscriptions.Subscription{}
		for _, subId := range m.Conf.Azure.Subscription {
			result, err := subscriptionsClient.Get(m.ctx, subId)
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
	kubeconf, err := clientcmd.BuildConfigFromFlags("", m.Conf.Kubernetes.Config)
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
		m.leaderElect()

		m.startIntervalSync(syncInterval)

		if m.Conf.Sync.Watch {
			m.startWatchSync()
		}
	}()
}

func (m *MsiOperator) leaderElect() {
	if m.Conf.Lease.Enabled {
		log.Info("trying to become leader")
		if m.Conf.Instance.Pod != nil && os.Getenv("POD_NAME") == "" {
			err := os.Setenv("POD_NAME", *m.Conf.Instance.Pod)
			if err != nil {
				log.Panic(err)
			}
		}

		time.Sleep(15 * time.Second)
		err := leader.Become(m.ctx, m.Conf.Lease.Name)
		if err != nil {
			log.Error(err, "Failed to retry for leader lock")
			os.Exit(1)
		}
		log.Info("aquired leader lock, continue")
	}
}

func (m *MsiOperator) startIntervalSync(syncInterval time.Duration) {
	go func() {
		for {
			m.run()
			time.Sleep(syncInterval)
		}
	}()
}

func (m *MsiOperator) startWatchSync() {
	go func() {
		for {
			gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
			watch, err := m.kubernetes.client.Resource(gvr).Watch(m.ctx, metav1.ListOptions{Watch: true})
			if err != nil {
				log.Panic(err)
			}

		watchLoop:
			for res := range watch.ResultChan() {
				switch strings.ToLower(string(res.Type)) {
				case "added":
					//namespace := res.Object.(*unstructured.Unstructured)
					m.run()
				case "error":
					break watchLoop
				}
			}

			log.Info("restarting namespace watch")
		}
	}()

	go func() {
		for {
			gvr := schema.GroupVersionResource{Group: K8sSchemeAzureIdentityBindingGroup, Version: K8sSchemeAzureIdentityBindingVersion, Resource: K8sSchemeAzureIdentityBindingResourcePlural}
			watch, err := m.kubernetes.client.Resource(gvr).Watch(m.ctx, metav1.ListOptions{Watch: true})
			if err != nil {
				log.Panic(err)
			}

		watchLoop:
			for res := range watch.ResultChan() {
				switch strings.ToLower(string(res.Type)) {
				case "added":
					//namespace := res.Object.(*unstructured.Unstructured)
					m.run()
				case "error":
					break watchLoop
				}
			}

			log.Info("restarting AzureIdentityBinding watch")
		}
	}()
}

func (m *MsiOperator) run() {
	if !m.lock.TryAcquire(1) {
		// already running
		return
	}
	defer m.lock.Release(1)

	log.Info("starting sync")
	overallStartTime := time.Now()

	for _, subscription := range m.azure.subscriptionList {
		subscriptionStartTime := time.Now()

		contextLogger := log.WithField("subscription", *subscription.DisplayName)

		contextLogger.Infof("sync Azure Subscription \"%s\" (%s)", to.String(subscription.DisplayName), to.String(subscription.SubscriptionID))
		err := m.upsertSubscription(contextLogger, &subscription)
		if err != nil {
			log.Error(err)
		}

		subscriptionSyncDuration := time.Since(subscriptionStartTime)
		m.prometheus.duration.WithLabelValues(*subscription.SubscriptionID).Set(subscriptionSyncDuration.Seconds())
		m.prometheus.lastSync.WithLabelValues(*subscription.SubscriptionID).SetToCurrentTime()
	}

	overallDuration := time.Since(overallStartTime)
	log.Infof("finished after %s", overallDuration.String())

	// lock next sync (keep up semaphore lock)
	time.Sleep(m.Conf.Sync.LockTime)
}

func (m *MsiOperator) upsertSubscription(contextLogger *log.Entry, subscription *subscriptions.Subscription) error {
	msiList, err := m.getAzureMsiList(subscription)
	if err != nil {
		return err
	}

	gvr := schema.GroupVersionResource{Group: K8sSchemeAzureIdentityGroup, Version: K8sSchemeAzureIdentityVersion, Resource: K8sSchemeAzureIdentityResourcePlural}
	for _, msiResource := range msiList {
		msiInfo, err := m.generateMsiKubernetesResourceInfo(msiResource)
		if err != nil {
			contextLogger.Error(err)
			continue
		}

		// add resource to log
		msiLogger := contextLogger.WithFields(log.Fields{
			"resource": to.String(msiInfo.AzureResourceId),
		})

		// check if namespace/resource was found
		if msiInfo.KubernetesNamespace == nil {
			msiLogger.Debugf("unable to generate Kubernetes namespace name for Azure MSI %v", to.String(msiResource.ID))
			continue
		}

		if msiInfo.KubernetesResourceName == nil {
			msiLogger.Debugf("unable to generate Kubernetes resource name for Azure MSI %v", to.String(msiResource.ID))
			continue
		}

		k8sResourceName := *msiInfo.KubernetesResourceName

		for _, k8sNamespace := range msiInfo.KubernetesNamespace {
			// add k8s info to log
			msiLogger = msiLogger.WithFields(log.Fields{
				"k8sNamespace": k8sNamespace,
				"k8sResource":  k8sResourceName,
			})

			k8sPodIdentity, _ := m.kubernetes.client.Resource(gvr).Namespace(k8sNamespace).Get(m.ctx, k8sResourceName, metav1.GetOptions{})
			if k8sPodIdentity != nil {
				// update
				msiLogger.Infof("updating AzureIdentity %v/%v", k8sNamespace, k8sResourceName)

				if err := m.applyMsiToK8sObject(msiResource, k8sPodIdentity); err != nil {
					msiLogger.Error(err)
					continue
				}

				_, err := m.kubernetes.client.Resource(gvr).Namespace(k8sNamespace).Update(m.ctx, k8sPodIdentity, metav1.UpdateOptions{})
				if err != nil {
					msiLogger.Error(err)
					m.prometheus.msiResourceErrors.WithLabelValues(*subscription.SubscriptionID, K8sSchemeAzureIdentityResourceSingular).Inc()
				} else {
					m.prometheus.msiResourcsSuccess.WithLabelValues(*subscription.SubscriptionID, K8sSchemeAzureIdentityResourceSingular).Inc()
				}
			} else {
				// create
				msiLogger.Infof("creating AzureIdentity %v/%v", k8sNamespace, k8sResourceName)

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

				if err := m.applyMsiToK8sObject(msiResource, k8sPodIdentity); err != nil {
					msiLogger.Error(err)
					continue
				}

				_, err := m.kubernetes.client.Resource(gvr).Namespace(k8sNamespace).Create(m.ctx, k8sPodIdentity, metav1.CreateOptions{})
				if err != nil {
					msiLogger.Error(err)
					m.prometheus.msiResourceErrors.WithLabelValues(*subscription.SubscriptionID, K8sSchemeAzureIdentityResourceSingular).Inc()
				} else {
					m.prometheus.msiResourcsSuccess.WithLabelValues(*subscription.SubscriptionID, K8sSchemeAzureIdentityResourceSingular).Inc()
				}
			}

			if m.Conf.AzureMsi.BindingSync {
				err := m.syncAzureIdentityToAzureIdentityBinding(msiLogger, msiInfo, k8sNamespace)
				if err != nil {
					msiLogger.Error(err)
				}
			}
		}
	}

	return nil
}

func (m *MsiOperator) syncAzureIdentityToAzureIdentityBinding(contextLogger *log.Entry, msiInfo MsiResourceInfo, k8sNamespace string) error {
	gvr := schema.GroupVersionResource{Group: K8sSchemeAzureIdentityBindingGroup, Version: K8sSchemeAzureIdentityBindingVersion, Resource: K8sSchemeAzureIdentityBindingResourcePlural}

	labelNameSubscription := m.labelName("subscription")
	labelValueSubscription := to.String(msiInfo.AzureSubscriptionId)

	labelNameResourceGroup := m.labelName("resourcegroup")
	labelValueResourceGroup := to.String(msiInfo.AzureResourceGroup)

	labelNameResourceName := m.labelName("name")
	labelValueResourceName := to.String(msiInfo.AzureResourceName)

	if validationErrors := validation.IsValidLabelValue(labelValueSubscription); len(validationErrors) != 0 {
		return fmt.Errorf("invalid label value \"%s\" for subscription: %v", labelValueSubscription, validationErrors)
	}

	if validationErrors := validation.IsValidLabelValue(labelValueResourceGroup); len(validationErrors) != 0 {
		return fmt.Errorf("invalid label value \"%s\" for resourcegroup: %v", labelValueResourceGroup, validationErrors)
	}

	if validationErrors := validation.IsValidLabelValue(labelValueResourceName); len(validationErrors) != 0 {
		return fmt.Errorf("invalid label value \"%s\" for resourcename: %v", labelValueResourceName, validationErrors)
	}

	listOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf(
			"%s=%s,%s=%s,%s=%s",
			labelNameSubscription, labelValueSubscription,
			labelNameResourceGroup, labelValueResourceGroup,
			labelNameResourceName, labelValueResourceName,
		),
	}

	list, err := m.kubernetes.client.Resource(gvr).Namespace(k8sNamespace).List(m.ctx, listOpts)
	if err != nil {
		return fmt.Errorf("failed to fetch AzureIdentityBinding from namespace \"%s\": %v", k8sNamespace, err)
	}

	if list != nil {
		for _, azureIdentityBinding := range list.Items {
			if err := unstructured.SetNestedField(azureIdentityBinding.Object, *msiInfo.KubernetesResourceName, "spec", "AzureIdentity"); err != nil {
				contextLogger.Warnf("failed to set object \"kind\" for AzureIdentityBinding \"%s/%s\": %v", k8sNamespace, azureIdentityBinding.GetName(), err)
				continue
			}

			_, err := m.kubernetes.client.Resource(gvr).Namespace(k8sNamespace).Update(m.ctx, &azureIdentityBinding, metav1.UpdateOptions{})
			if err != nil {
				contextLogger.Warnf("unable to sync AzureIdentity \"%[1]s/%[3]s\" to AzureIdentityBinding \"%[1]s/%[2]s\" : %[4]v", k8sNamespace, azureIdentityBinding.GetName(), *msiInfo.KubernetesResourceName, err)
				m.prometheus.msiResourceErrors.WithLabelValues(*msiInfo.AzureSubscriptionId, K8sSchemeAzureIdentityBindingResourceSingular).Inc()
			} else {
				contextLogger.Infof("successfully synced AzureIdentity \"%[1]s/%[3]s\" to AzureIdentityBinding \"%[1]s/%[2]s\"", k8sNamespace, azureIdentityBinding.GetName(), *msiInfo.KubernetesResourceName)
				m.prometheus.msiResourcsSuccess.WithLabelValues(*msiInfo.AzureSubscriptionId, K8sSchemeAzureIdentityBindingResourceSingular).Inc()
			}
		}
	}

	return nil
}

func (m *MsiOperator) generateMsiKubernetesResourceInfo(msi *msi.Identity) (msiInfo MsiResourceInfo, err error) {
	msiInfo = MsiResourceInfo{
		AzureResourceId: to.StringPtr(strings.ToLower(to.String(msi.ID))),
	}

	resourceInfo, parseErr := azure.ParseResourceID(*msi.ID)
	if parseErr != nil {
		err = parseErr
		return
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
		Id:             to.String(msi.ID),
		Name:           to.String(msi.Name),
		Location:       to.String(msi.Location),
		ResourceGroup:  resourceInfo.ResourceGroup,
		SubscriptionId: resourceInfo.SubscriptionID,
		ClientId:       msi.ClientID.String(),
		TenantId:       msi.TenantID.String(),
		PrincipalID:    msi.PrincipalID.String(),
		Tags:           to.StringMap(msi.Tags),
		Type:           to.String(msi.Type),
	}

	msiInfo.Msi = msi
	msiInfo.AzureResourceName = to.StringPtr(strings.ToLower(resourceInfo.ResourceName))
	msiInfo.AzureResourceGroup = to.StringPtr(strings.ToLower(resourceInfo.ResourceGroup))
	msiInfo.AzureSubscriptionId = to.StringPtr(strings.ToLower(resourceInfo.SubscriptionID))

	resNameBuf := &bytes.Buffer{}
	if err := m.msi.resourceNameTemplate.Execute(resNameBuf, templateData); err != nil {
		log.Panic(err)
	}
	if val := resNameBuf.String(); val != "" {
		msiInfo.KubernetesResourceName = &val
	}

	namespaceBuf := &bytes.Buffer{}
	if err := m.msi.namespaceTemplate.Execute(namespaceBuf, templateData); err != nil {
		log.Panic(err)
	}
	if val := namespaceBuf.String(); val != "" {
		for _, namespace := range strings.Split(val, ",") {
			namespace = strings.ToLower(strings.TrimSpace(namespace))

			if contains(m.Conf.Kubernetes.NamespaceIgnore, namespace) {
				continue
			}

			msiInfo.KubernetesNamespace = append(
				msiInfo.KubernetesNamespace,
				strings.TrimSpace(namespace),
			)
		}
	}

	return
}

func (m *MsiOperator) applyMsiToK8sObject(msi *msi.Identity, k8sResource *unstructured.Unstructured) error {
	msiResourceId := to.String(msi.ID)
	msiClientId := msi.ClientID.String()

	resourceInfo, err := azure.ParseResourceID(msiResourceId)
	if err != nil {
		return err
	}

	// ensure lowercase
	msiResourceId = strings.ToLower(msiResourceId)
	resourceInfo.SubscriptionID = strings.ToLower(resourceInfo.SubscriptionID)
	resourceInfo.ResourceGroup = strings.ToLower(resourceInfo.ResourceGroup)
	resourceInfo.ResourceName = strings.ToLower(resourceInfo.ResourceName)

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
	if m.Conf.AzureMsi.Namespaced {
		if err := unstructured.SetNestedField(k8sResource.Object, "namespaced", "metadata", "annotations", "aadpodidentity.k8s.io/Behavior"); err != nil {
			return fmt.Errorf("failed to set metadata.annotations[aadpodidentity.k8s.io/Behavior] value: %v", err)
		}
	} else {
		unstructured.RemoveNestedField(k8sResource.Object, "metadata", "annotations", "aadpodidentity.k8s.io/Behavior")
	}

	// labels
	labelName := m.labelName("subscription")
	if err := unstructured.SetNestedField(k8sResource.Object, resourceInfo.SubscriptionID, "metadata", "labels", labelName); err != nil {
		return fmt.Errorf("failed to set metadata.labels[%v] value: %v", labelName, err)
	}

	labelName = m.labelName("resourcegroup")
	if err := unstructured.SetNestedField(k8sResource.Object, resourceInfo.ResourceGroup, "metadata", "labels", labelName); err != nil {
		return fmt.Errorf("failed to set metadata.labels[%v] value: %v", labelName, err)
	}

	labelName = m.labelName("name")
	if err := unstructured.SetNestedField(k8sResource.Object, resourceInfo.ResourceName, "metadata", "labels", labelName); err != nil {
		return fmt.Errorf("failed to set metadata.labels[%v] value: %v", labelName, err)
	}

	return nil
}

func (m *MsiOperator) getAzureMsiList(subscription *subscriptions.Subscription) (ret []*msi.Identity, err error) {
	client := msi.NewUserAssignedIdentitiesClientWithBaseURI(m.azure.environment.ResourceManagerEndpoint, *subscription.SubscriptionID)
	client.Authorizer = m.azure.authorizer

	list, azureErr := client.ListBySubscriptionComplete(m.ctx)
	if azureErr != nil {
		err = azureErr
		return
	}

	for list.NotDone() {
		result := list.Value()
		ret = append(ret, &result)
		if list.NextWithContext(m.ctx) != nil {
			break
		}
	}

	return
}

func (m *MsiOperator) labelName(name string) string {
	return fmt.Sprintf(m.Conf.Kubernetes.LabelFormat, name)
}
