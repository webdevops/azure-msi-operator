package operator

import (
	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"sync"
)

type (
	MsiResourceList struct {
		list       []MsiResourceInfo
		uncommited []MsiResourceInfo
		lock       sync.Mutex
	}

	MsiResourceInfo struct {
		Resource               *msi.Identity
		AzureResourceId        *string
		AzureResourceName      *string
		AzureResourceGroup     *string
		AzureSubscriptionId    *string
		KubernetesResourceName *string
		KubernetesNamespace    []string
	}
)

func NewMsiResourceList() *MsiResourceList {
	return &MsiResourceList{
		list:       []MsiResourceInfo{},
		uncommited: []MsiResourceInfo{},
	}
}

func (m *MsiResourceList) Add(val MsiResourceInfo) {
	m.uncommited = append(m.uncommited, val)
}

func (m *MsiResourceList) Clean() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.uncommited = []MsiResourceInfo{}
}

func (m *MsiResourceList) Commit() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.list = m.uncommited
	m.uncommited = []MsiResourceInfo{}
}

func (m *MsiResourceList) GetList() []MsiResourceInfo {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.list
}
