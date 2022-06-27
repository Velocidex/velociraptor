package orgs

import (
	"context"
	"errors"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
)

type ClientOrgManager struct {
	services   services.ServiceContainer
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
	services := &ServiceContainer{}
	services.journal, err = journal.NewJournalService(
		ctx, wg, config_obj)
	if err != nil {
		return err
	}

	self.services = services
	return nil
}

func StartClientOrgManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	service := &ClientOrgManager{
		config_obj: config_obj,
	}
	services.RegisterOrgManager(service)

	return service.Start(ctx, wg, config_obj)
}
