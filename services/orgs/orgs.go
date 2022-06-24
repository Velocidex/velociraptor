package orgs

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

type OrgManager struct {
	mu         sync.Mutex
	config_obj *config_proto.Config

	// Each org has a separate config object.
	org_configs     map[string]*config_proto.Config
	org_id_by_nonce map[string]string
}

func (self *OrgManager) GetOrgConfig(org_id string) (*config_proto.Config, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result, pres := self.org_configs[org_id]
	if !pres {
		return nil, services.NotFoundError
	}
	return result, nil
}

func (self *OrgManager) OrgIdByNonce(nonce string) (string, error) {
	result, pres := self.org_id_by_nonce[nonce]
	if !pres {
		return "", services.NotFoundError
	}
	return result, nil
}

func (self *OrgManager) CreateNewOrg() (string, error) {
	return "", services.NotFoundError
}

func (self *OrgManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Org Manager service.")

	// Start syncing the mutation_manager
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()

	return nil
}

func StartOrgManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	service := &OrgManager{
		config_obj:      config_obj,
		org_configs:     make(map[string]*config_proto.Config),
		org_id_by_nonce: make(map[string]string),
	}
	services.RegisterOrgManager(service)

	return service.Start(ctx, config_obj, wg)
}
