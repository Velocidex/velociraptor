package orgs

import (
	"context"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

type OrgContext struct {
	record     *api_proto.OrgRecord
	config_obj *config_proto.Config
	service    services.ServiceContainer
}

type OrgManager struct {
	mu sync.Mutex

	ctx context.Context
	wg  *sync.WaitGroup

	// The base global config object
	config_obj *config_proto.Config

	// Each org has a separate config object.
	orgs            map[string]*OrgContext
	org_id_by_nonce map[string]string
}

func (self *OrgManager) ListOrgs() []*api_proto.OrgRecord {
	result := []*api_proto.OrgRecord{}
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, item := range self.orgs {
		result = append(result, proto.Clone(item.record).(*api_proto.OrgRecord))
	}

	return result
}

func (self *OrgManager) GetOrgConfig(org_id string) (*config_proto.Config, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// An empty org id corresponds to the root org.
	if org_id == "" {
		return self.config_obj, nil
	}

	result, pres := self.orgs[org_id]
	if !pres {
		return nil, services.NotFoundError
	}
	return result.config_obj, nil
}

func (self *OrgManager) GetOrg(org_id string) (*api_proto.OrgRecord, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result, pres := self.orgs[org_id]
	if !pres {
		return nil, services.NotFoundError
	}
	return result.record, nil
}

func (self *OrgManager) OrgIdByNonce(nonce string) (string, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Nonce corresponds to the root config
	if self.config_obj.Client != nil &&
		self.config_obj.Client.Nonce == nonce {
		return "", nil
	}

	result, pres := self.org_id_by_nonce[nonce]
	if !pres {
		return "", services.NotFoundError
	}
	return result, nil
}

func (self *OrgManager) CreateNewOrg(name string) (
	*api_proto.OrgRecord, error) {

	org_record := &api_proto.OrgRecord{
		Name:  name,
		OrgId: NewOrgId(),
		Nonce: NewNonce(),
	}

	err := self.startOrg(org_record)
	if err != nil {
		return nil, err
	}

	org_path_manager := paths.NewOrgPathManager(
		org_record.OrgId)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	err = db.SetSubject(self.config_obj,
		org_path_manager.Path(), org_record)
	return org_record, err
}

func (self *OrgManager) makeNewConfigObj(
	record *api_proto.OrgRecord) *config_proto.Config {

	result := proto.Clone(self.config_obj).(*config_proto.Config)

	// The Root org is untouched.
	if record.OrgId == "" {
		return result
	}

	if result.Client != nil {
		result.OrgId = record.OrgId
		result.OrgName = record.Name
		result.Client.Nonce = record.Nonce
	}

	if result.Datastore != nil {
		if result.Datastore.Location != "" {
			result.Datastore.Location += "/orgs/" + record.OrgId
		}
		if result.Datastore.FilestoreDirectory != "" {
			result.Datastore.FilestoreDirectory += "/orgs/" + record.OrgId
		}
	}

	return result
}

func (self *OrgManager) Scan() error {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	children, err := db.ListChildren(
		self.config_obj, paths.ORGS_ROOT)
	if err != nil {
		return err
	}

	for _, org_path := range children {
		org_id := org_path.Base()
		org_path_manager := paths.NewOrgPathManager(org_id)
		org_record := &api_proto.OrgRecord{}
		err := db.GetSubject(self.config_obj,
			org_path_manager.Path(), org_record)
		if err != nil {
			continue
		}

		_, err = self.GetOrgConfig(org_id)
		if err != nil {
			err = self.startOrg(org_record)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (self *OrgManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Org Manager service.")

	// First start all services for the root org
	self.startOrg(&api_proto.OrgRecord{
		OrgId: "",
		Name:  "<root org>",
		Nonce: config_obj.Client.Nonce,
	})

	// Do first scan inline so we have valid data on exit.
	err := self.Scan()
	if err != nil {
		return err
	}

	// Start syncing the mutation_manager
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(10 * time.Second):
				self.Scan()
			}
		}

	}()

	return nil
}

func StartOrgManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	service := &OrgManager{
		config_obj: config_obj,
		ctx:        ctx,
		wg:         wg,

		orgs:            make(map[string]*OrgContext),
		org_id_by_nonce: make(map[string]string),
	}
	services.RegisterOrgManager(service)

	return service.Start(ctx, config_obj, wg)
}
