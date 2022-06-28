package orgs

import (
	"errors"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/broadcast"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/services/indexing"
	"www.velocidex.com/golang/velociraptor/services/interrogation"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
)

type ServiceContainer struct {
	mu sync.Mutex

	journal             services.JournalService
	client_info_manager services.ClientInfoManager
	indexer             services.Indexer
	broadcast           services.BroadcastService
	inventory           services.Inventory
}

func (self ServiceContainer) Indexer() (services.Indexer, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.indexer == nil {
		return nil, errors.New("Indexing service not initialized")
	}

	return self.indexer, nil
}

func (self ServiceContainer) Journal() (services.JournalService, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.journal == nil {
		return nil, errors.New("Journal service not ready")
	}
	return self.journal, nil
}

func (self ServiceContainer) ClientInfoManager() (services.ClientInfoManager, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.client_info_manager == nil {
		return nil, errors.New("Client Info Manager not ready")
	}
	return self.client_info_manager, nil
}

func (self ServiceContainer) Inventory() (services.Inventory, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.inventory == nil {
		return nil, errors.New("Inventory Manager not ready")
	}
	return self.inventory, nil
}

func (self ServiceContainer) BroadcastService() (services.BroadcastService, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.broadcast == nil {
		return nil, errors.New("Broadcast Service not ready")
	}
	return self.broadcast, nil
}

// Start all the services for the org and install it in the
// manager. This function is used both in the client and the server to
// start all the needed services.
func (self *OrgManager) startOrg(org_record *api_proto.OrgRecord) (err error) {
	org_config := self.makeNewConfigObj(org_record)
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Starting services for %v", services.GetOrgName(org_config))

	self.mu.Lock()
	org_id := org_record.OrgId

	service_container := &ServiceContainer{}
	self.orgs[org_id] = &OrgContext{
		record:     org_record,
		config_obj: org_config,
		service:    service_container,
	}
	self.org_id_by_nonce[org_record.Nonce] = org_id
	self.mu.Unlock()

	// Now start the services for this org. Services depend on other
	// services so they need to be accessible as soon as they are
	// ready.
	j, err := journal.NewJournalService(
		self.ctx, self.wg, org_config)
	if err != nil {
		return err
	}
	service_container.mu.Lock()
	service_container.journal = j
	service_container.broadcast = broadcast.NewBroadcastService(org_config)
	service_container.mu.Unlock()

	i, err := inventory.NewInventoryService(
		self.ctx, self.wg, org_config)
	if err != nil {
		return err
	}

	service_container.mu.Lock()
	service_container.inventory = i
	service_container.mu.Unlock()

	err = interrogation.StartInterrogationService(
		self.ctx, self.wg, org_config)

	if err != nil {
		return err
	}

	c := client_info.NewClientInfoManager(org_config)
	err = c.Start(self.ctx, org_config, self.wg)
	if err != nil {
		return err
	}

	service_container.mu.Lock()
	service_container.client_info_manager = c
	service_container.mu.Unlock()

	inv, err := indexing.NewIndexingService(self.ctx, self.wg, org_config)
	if err != nil {
		return err
	}

	service_container.mu.Lock()
	service_container.indexer = inv
	service_container.mu.Unlock()

	return err
}

func (self *OrgManager) Services(org_id string) services.ServiceContainer {
	self.mu.Lock()
	defer self.mu.Unlock()

	service_container, pres := self.orgs[org_id]
	if !pres {
		return &ServiceContainer{}
	}
	return service_container.service
}
