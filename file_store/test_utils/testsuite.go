package test_utils

import (
	"context"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/yaml/v2"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/indexing"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

var (
	// These artifacts are normally defined as assets but we define
	// them separately for tests.
	definitions = []string{`
name: Server.Internal.HuntModification
type: INTERNAL
`, `
name: Server.Audit.Logs
type: INTERNAL
`, `
name: Server.Internal.ClientInfoSnapshot
type: INTERNAL
`, `
name: Server.Internal.ClientInfo
type: INTERNAL
`, `
name: Server.Internal.ClientDelete
type: INTERNAL
`, `
name: Server.Internal.Label
type: INTERNAL
`, `
name: Server.Internal.UserManager
type: INTERNAL
`, `
name: Server.Internal.Notifications
type: INTERNAL
`, `
name: Server.Internal.Interrogation
type: INTERNAL
`, `
name: Server.Internal.Ping
type: INTERNAL
`, `
name: System.Flow.Completion
type: CLIENT_EVENT
`, `
name: System.Hunt.Creation
type: SERVER_EVENT
`, `
name: Server.Internal.ArtifactModification
type: SERVER_EVENT
`, `
name: Server.Internal.FrontendMetrics
type: INTERNAL
`, `
name: Server.Monitor.Health
type: SERVER_EVENT
`, `
name: Generic.Client.Stats
type: CLIENT_EVENT
`, `
name: System.Hunt.Participation
type: INTERNAL
`, `
name: System.Upload.Completion
type: SERVER
`, `
name: Server.Internal.Enrollment
type: INTERNAL
`, `
name: Server.Internal.MasterRegistrations
type: INTERNAL
`, `
name: Server.Internal.ClientTasks
type: INTERNAL
`, `
name: Generic.Client.Info
type: CLIENT
sources:
- precondition: SELECT * FROM info()
  query: SELECT * FROM info()
  name: BasicInformation

- precondition: SELECT * FROM info()
  query: SELECT * FROM info()
  name: Users
`, `
name: Artifact.With.Parameters
parameters:
- name: Param1
sources:
- query: SELECT * FROM info()
`,
	}
)

type TestSuite struct {
	suite.Suite
	ConfigObj *config_proto.Config
	Ctx       context.Context
	cancel    func()
	Sm        *services.Service
	Wg        *sync.WaitGroup

	Services *orgs.ServiceContainer
}

func (self *TestSuite) CreateClient(client_id string) {
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: client_id,
		}})
	assert.NoError(self.T(), err)
}

func (self *TestSuite) CreateFlow(client_id, flow_id string) {
	defer utils.SetFlowIdForTests(flow_id)()

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(self.T(), err)

	_, err = launcher.ScheduleArtifactCollection(
		self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		&flows_proto.ArtifactCollectorArgs{
			ClientId:  client_id,
			Artifacts: []string{"Generic.Client.Info"},
		}, nil)
	assert.NoError(self.T(), err)
}

func (self *TestSuite) LoadConfig() *config_proto.Config {
	os.Setenv("VELOCIRAPTOR_LITERAL_CONFIG", SERVER_CONFIG)
	config_obj, err := new(config.Loader).
		WithEnvLiteralLoader(constants.VELOCIRAPTOR_LITERAL_CONFIG).
		WithRequiredFrontend().
		WithWriteback().WithVerbose(true).
		LoadAndValidate()
	require.NoError(self.T(), err)

	config_obj.Frontend.DoNotCompressArtifacts = true

	return config_obj
}

func (self *TestSuite) SetupTest() {
	services.AllowFrontendPlugins.Store(true)

	if self.ConfigObj == nil {
		self.ConfigObj = self.LoadConfig()
	}

	err := datastore.SetGlobalDatastore(context.Background(),
		self.ConfigObj.Datastore.Implementation, self.ConfigObj)
	assert.NoError(self.T(), err)

	self.LoadArtifactsIntoConfig(definitions)

	// Start essential services.
	self.Ctx, self.cancel = context.WithTimeout(context.Background(), time.Second*60)
	self.Sm = services.NewServiceManager(self.Ctx, self.ConfigObj)
	self.Wg = &sync.WaitGroup{}

	err = orgs.StartTestOrgManager(
		self.Ctx, self.Wg, self.ConfigObj, self.Services)
	require.NoError(self.T(), err)

	// Wait here until the indexer is all ready.
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		indexer, err := services.GetIndexer(self.ConfigObj)
		assert.NoError(self.T(), err)
		return indexer.(*indexing.Indexer).IsReady()
	})
}

func (self *TestSuite) LoadArtifacts(definitions ...string) services.Repository {
	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)

	for _, definition := range definitions {
		_, err := repository.LoadYaml(definition,
			services.ArtifactOptions{
				ValidateArtifact:  false,
				ArtifactIsBuiltIn: true})
		assert.NoError(self.T(), err)
	}
	return repository
}

// Parse the definitions and add them to the config so they will be
// loaded by the repository manager.
func (self *TestSuite) LoadArtifactsIntoConfig(definitions []string) {
	if self.ConfigObj.Autoexec == nil {
		self.ConfigObj.Autoexec = &config_proto.AutoExecConfig{}
	}

	existing_artifacts := make(map[string]*artifacts_proto.Artifact)
	for _, def := range self.ConfigObj.Autoexec.ArtifactDefinitions {
		def.Raw = ""
		existing_artifacts[def.Name] = def
	}

	for _, definition := range definitions {
		artifact := &artifacts_proto.Artifact{}
		err := yaml.Unmarshal([]byte(definition), artifact)
		assert.NoError(self.T(), err)
		_, pres := existing_artifacts[artifact.Name]
		if !pres {
			existing_artifacts[artifact.Name] = artifact
		}
	}
	artifacts := []*artifacts_proto.Artifact{}
	for _, v := range existing_artifacts {
		artifacts = append(artifacts, v)
	}

	// Sort for stability
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Name < artifacts[j].Name
	})
	self.ConfigObj.Autoexec.ArtifactDefinitions = artifacts

}

func (self *TestSuite) LoadArtifactFiles(paths ...string) {
	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	global_repo, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	for _, p := range paths {
		fd, err := os.Open(p)
		assert.NoError(self.T(), err)

		def, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
		assert.NoError(self.T(), err)

		_, err = global_repo.LoadYaml(string(def),
			services.ArtifactOptions{
				ValidateArtifact:  true,
				ArtifactIsBuiltIn: true})
		assert.NoError(self.T(), err)
	}
}

func (self *TestSuite) TearDownTest() {
	if self.cancel != nil {
		self.cancel()
	}
	if self.Sm != nil {
		self.Sm.Close()
	}

	// These may not be memory based in the test switched to other
	// data stores.
	file_store_factory, ok := file_store.GetFileStore(
		self.ConfigObj).(*memory.MemoryFileStore)
	if ok {
		file_store_factory.Clear()
	}

	db := GetMemoryDataStore(self.T(), self.ConfigObj)
	if db != nil {
		db.Clear()
	}
}
