package orgs

import (
	"context"
	"errors"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/repository"
)

type ClientOrgManager struct {
	services   *ServiceContainer
	config_obj *config_proto.Config
}

func (self ClientOrgManager) GetOrgConfig(org_id string) (*config_proto.Config, error) {
	return self.config_obj, nil
}

func (self ClientOrgManager) OrgIdByNonce(nonce string) (string, error) {
	return "", nil
}

func (self ClientOrgManager) CreateNewOrg(name string) (*api_proto.OrgRecord, error) {
	return nil, errors.New("Not implemented")
}

func (self ClientOrgManager) ListOrgs() []*api_proto.OrgRecord {
	return nil
}
func (self ClientOrgManager) GetOrg(org_id string) (*api_proto.OrgRecord, error) {
	return nil, errors.New("Not implemented")
}

func (self ClientOrgManager) Services(org_id string) services.ServiceContainer {
	return self.services
}

func (self *ClientOrgManager) Start(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (err error) {

	// Now start the services for this org.
	j, err := journal.NewJournalService(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	self.services.mu.Lock()
	self.services.journal = j
	self.services.mu.Unlock()

	repo_manager, err := repository.NewRepositoryManager(ctx, wg, config_obj)
	if err != nil {
		return err
	}

	self.services.mu.Lock()
	self.services.repository = repo_manager
	self.services.mu.Unlock()

	return nil
}

func StartClientOrgManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	service := &ClientOrgManager{
		config_obj: config_obj,
		services:   &ServiceContainer{},
	}
	services.RegisterOrgManager(service)

	return service.Start(ctx, wg, config_obj)
}
