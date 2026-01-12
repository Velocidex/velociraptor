package services

import (
	"context"
	"errors"
	"fmt"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

const (
	ROOT_ORG_ID   = "root"
	ROOT_ORG_NAME = "<root>"
	RandomNonce   = ""
)

var (
	mu          sync.Mutex
	org_manager OrgManager

	OrgNotFoundError = errors.New("Org not found")
)

// Currently the org manager is the only binary wide global - all
// other services use the org manager to find other services.
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

// hold all the services for each org
type ServiceContainer interface {
	FrontendManager() (FrontendManager, error)
	Journal() (JournalService, error)
	ClientInfoManager() (ClientInfoManager, error)
	Indexer() (Indexer, error)
	BroadcastService() (BroadcastService, error)
	Inventory() (Inventory, error)
	VFSService() (VFSService, error)
	Labeler() (Labeler, error)
	RepositoryManager() (RepositoryManager, error)
	HuntDispatcher() (IHuntDispatcher, error)
	Launcher() (Launcher, error)
	NotebookManager() (NotebookManager, error)
	ClientEventManager() (ClientEventTable, error)
	ServerEventManager() (ServerEventManager, error)
	ServerArtifactRunner() (ServerArtifactRunner, error)
	Notifier() (Notifier, error)
	ACLManager() (ACLManager, error)
	AuditManager() (AuditManager, error)
	Scheduler() (Scheduler, error)
	SecretsService() (SecretsService, error)
	BackupService() (BackupService, error)
	ExportManager() (ExportManager, error)
	DocManager() (DocManager, error)
}

// The org manager manages multi-tenancies.
type OrgManager interface {
	GetOrgConfig(org_id string) (*config_proto.Config, error)
	OrgIdByNonce(nonce string) (string, error)
	CreateNewOrg(name, id, nonce string) (*api_proto.OrgRecord, error)
	ListOrgs() []*api_proto.OrgRecord
	GetOrg(org_id string) (*api_proto.OrgRecord, error)
	DeleteOrg(ctx context.Context, principal, org_id string) error

	// The manager is responsible for running multiple services - one
	// for each org. This ensures org services are separated out and
	// one org can not access data from another org.
	Services(org_id string) ServiceContainer
}

func GetOrgName(config_obj *config_proto.Config) string {
	org_id := config_obj.OrgId
	org_name := config_obj.OrgName

	if org_id == "" {
		org_id = ROOT_ORG_ID
		org_name = ROOT_ORG_NAME
	}

	return fmt.Sprintf("Org %v (%v)", org_name, org_id)
}
