package services

import (
	"errors"
	"fmt"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	org_manager OrgManager

	NotFoundError = errors.New("Org not found")
)

func GetOrgManager() (OrgManager, error) {
	mu.Lock()
	defer mu.Unlock()

	if org_manager == nil {
		return nil, errors.New("Org Manager not initialized")
	}

	return org_manager, nil
}

func RegisterOrgManager(m OrgManager) {
	mu.Lock()
	defer mu.Unlock()

	org_manager = m
}

type ServiceContainer interface {
	Journal() (JournalService, error)
	ClientInfoManager() (ClientInfoManager, error)
}

// The org manager manages multi-tenancies.
type OrgManager interface {
	GetOrgConfig(org_id string) (*config_proto.Config, error)
	OrgIdByNonce(nonce string) (string, error)
	CreateNewOrg(name string) (*api_proto.OrgRecord, error)
	ListOrgs() []*api_proto.OrgRecord
	GetOrg(org_id string) (*api_proto.OrgRecord, error)

	// The manager is responsible for running multiple services - one
	// for each org. This ensures org services are separated out and
	// one org can not access data from another org.
	Services(org_id string) ServiceContainer
}

func GetOrgName(config_obj *config_proto.Config) string {
	if config_obj.OrgId == "" {
		return "Root Org"
	}

	return fmt.Sprintf("Org %v (%v)",
		config_obj.OrgName, config_obj.OrgId)
}
