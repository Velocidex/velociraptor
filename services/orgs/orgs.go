package orgs

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	orgStartCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "org_start_count",
			Help: "Number of orgs start started their services",
		})
)

type OrgContext struct {
	record     *api_proto.OrgRecord
	config_obj *config_proto.Config
	service    services.ServiceContainer

	// Manages the lifetime of the org's services.
	sm *services.Service
}

type OrgManager struct {
	mu sync.Mutex

	// The root org's ctx and wg
	ctx context.Context

	// We keep track of each org's services using its own wg and
	// control overall lifetime using our parent's wg. This allows us
	// to cancel each org's sevices independently.
	parent_wg *sync.WaitGroup

	// The base global config object
	config_obj *config_proto.Config

	// Each org has a separate config object.
	orgs            map[string]*OrgContext
	org_id_by_nonce map[string]string

	NextOrgIdForTesting *string
}

func (self *OrgManager) ListOrgs() []*api_proto.OrgRecord {
	result := []*api_proto.OrgRecord{}
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, item := range self.orgs {
		copy := proto.Clone(item.record).(*api_proto.OrgRecord)
		if utils.IsRootOrg(copy.Id) {
			copy.Id = services.ROOT_ORG_ID
			copy.Name = services.ROOT_ORG_NAME
			if self.config_obj.Client != nil {
				copy.Nonce = self.config_obj.Client.Nonce
			}
		}
		result = append(result, copy)
	}

	// Sort orgs by names
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

func (self *OrgManager) GetOrgConfig(org_id string) (*config_proto.Config, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	org_id = utils.NormalizedOrgId(org_id)

	// An empty org id corresponds to the root org.
	if utils.IsRootOrg(org_id) {
		return self.config_obj, nil
	}

	result, pres := self.orgs[org_id]
	if !pres {
		return nil, services.OrgNotFoundError
	}
	return result.config_obj, nil
}

func (self *OrgManager) GetOrg(org_id string) (*api_proto.OrgRecord, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	org_id = utils.NormalizedOrgId(org_id)

	result, pres := self.orgs[org_id]
	if !pres {
		return nil, services.OrgNotFoundError
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
		return "", services.OrgNotFoundError
	}
	return result, nil
}

func (self *OrgManager) CreateNewOrg(name, id, nonce string) (
	*api_proto.OrgRecord, error) {

	if id == "" {
		id = self.NewOrgId()
	}

	id = utils.NormalizedOrgId(id)

	_, err := self.GetOrg(id)
	if err == nil {
		return nil, errors.New("CreateNewOrg: Org ID already in use")
	}

	if nonce == services.RandomNonce {
		nonce = NewNonce()
	}

	org_record := &api_proto.OrgRecord{
		Name:  name,
		Id:    id,
		Nonce: nonce,
	}

	// Check if the org already exists
	self.mu.Lock()
	_, pres := self.orgs[id]
	self.mu.Unlock()

	if pres {
		return nil, errors.New("Org ID already exists")
	}

	org_path_manager := paths.NewOrgPathManager(
		org_record.Id)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	// Must appear immediately to ensure the org appears before we can
	// scan for it.
	err = db.SetSubjectWithCompletion(self.config_obj,
		org_path_manager.Path(), org_record, utils.SyncCompleter)
	if err != nil {
		return nil, err
	}

	return org_record, self.startOrg(org_record)
}

func (self *OrgManager) makeNewConfigObj(
	record *api_proto.OrgRecord) *config_proto.Config {

	// The root org carries the real global config, but other orgs'
	// config will be derived from the root org.
	result := self.config_obj
	if !utils.IsRootOrg(record.Id) {
		result = proto.Clone(self.config_obj).(*config_proto.Config)
	}

	result.OrgId = utils.NormalizedOrgId(record.Id)
	result.OrgName = record.Name

	if result.Client != nil {
		// Client config does not leak org id! We use the nonce to tie
		// org id back to the org.
		result.Client.Nonce = record.Nonce
	}

	// Adjust the datastore directories to point at a per-org
	// location:

	// The root location remains at the top level but suborgs will
	// live in <fs>/orgs/<orgid>
	if result.Datastore != nil && !utils.IsRootOrg(record.Id) {
		result.Datastore.Location = filepath.Join(
			result.Datastore.Location, "orgs", record.Id)
		result.Datastore.FilestoreDirectory = filepath.Join(
			result.Datastore.FilestoreDirectory, "orgs", record.Id)
	}

	return result
}

func (self *OrgManager) Scan() error {
	existing := make(map[string]bool)
	for _, o := range self.ListOrgs() {
		existing[o.Id] = true
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	children, err := db.ListChildren(self.config_obj, paths.ORGS_ROOT)
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

		// Read existing records for backwards compatibility
		if org_record.OrgId != "" && org_record.Id == "" {
			org_record.Id = org_record.OrgId
		}

		delete(existing, org_id)

		if org_record.Id == "" || org_record.Nonce == "" {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Info("<yellow>Org is corrupted %v</>", org_id)
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

	// Now shut down the orgs that were removed
	for org_id := range existing {
		org_id = utils.NormalizedOrgId(org_id)

		// Do not remove the root org
		if utils.IsRootOrg(org_id) {
			continue
		}

		self.mu.Lock()
		org_context, pres := self.orgs[org_id]
		self.mu.Unlock()
		if pres {
			org_context.sm.Close()

			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Info("<yellow>Removing org %v</>", org_id)

			self.mu.Lock()
			delete(self.orgs, org_id)
			delete(self.org_id_by_nonce, org_id)
			self.mu.Unlock()
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

	nonce := ""
	if config_obj.Client != nil {
		nonce = config_obj.Client.Nonce
	}

	// First start all services for the root org
	err := self.startOrg(&api_proto.OrgRecord{
		Id:    services.ROOT_ORG_ID,
		Name:  services.ROOT_ORG_NAME,
		Nonce: nonce,
	})
	if err != nil {
		return err
	}

	// If a datastore is not configured we are running on the client
	// or as a tool so we dont need to scan for new orgs.
	if config_obj.Datastore == nil {
		return nil
	}

	// Do first scan inline so we have valid data on exit.
	err = self.Scan()
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

			case <-time.After(utils.Jitter(10 * time.Second)):
				err := self.Scan()
				if err != nil {
					logger.Error("<red>OrgManager Scan</> %v", err)
				}
			}
		}

	}()

	return nil
}

func NewOrgManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.OrgManager, error) {

	service := &OrgManager{
		config_obj: config_obj,
		ctx:        ctx,
		parent_wg:  wg,

		orgs:            make(map[string]*OrgContext),
		org_id_by_nonce: make(map[string]string),
	}

	// Usually only one org manager exists at one time. However for
	// the "gui" command this may be invoked multiple times.
	_, err := services.GetOrgManager()
	if err != nil {
		services.RegisterOrgManager(service)
	}

	return service, service.Start(ctx, config_obj, wg)
}
