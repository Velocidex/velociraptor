package services

import (
	"errors"

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

// The org manager manages multi-tenancies.
type OrgManager interface {
	GetOrgConfig(org_id string) (*config_proto.Config, error)
	OrgIdByNonce(nonce string) (string, error)
	CreateNewOrg() (string, error)
}
