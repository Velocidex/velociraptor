package orgs

import (
	"context"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

type TestOrgManager struct {
	*OrgManager
}

func (self *TestOrgManager) Start(
	ctx context.Context,
	org_config *config_proto.Config,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(org_config, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Test Org Manager service.")

	self.mu.Lock()
	service_container := &ServiceContainer{}
	org_context := &OrgContext{
		record:     &api_proto.OrgRecord{},
		config_obj: org_config,
		service:    service_container,
	}
	self.orgs[""] = org_context
	self.mu.Unlock()

	return self.startOrg(org_context.record)
}

func StartTestOrgManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	service := &TestOrgManager{&OrgManager{
		config_obj: config_obj,
		ctx:        ctx,
		wg:         wg,

		orgs:            make(map[string]*OrgContext),
		org_id_by_nonce: make(map[string]string),
	}}
	services.RegisterOrgManager(service)

	return service.Start(ctx, config_obj, wg)
}
