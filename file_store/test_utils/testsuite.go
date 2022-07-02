package test_utils

import (
	"context"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/yaml/v2"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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
name: Server.Internal.ClientDelete
type: INTERNAL
`, `
name: Server.Internal.Label
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

func (self *TestSuite) LoadConfig() *config_proto.Config {
	os.Setenv("VELOCIRAPTOR_CONFIG", SERVER_CONFIG)
	config_obj, err := new(config.Loader).
		WithEnvLiteralLoader("VELOCIRAPTOR_CONFIG").WithRequiredFrontend().
		WithWriteback().WithVerbose(true).
		LoadAndValidate()
	require.NoError(self.T(), err)

	config_obj.Frontend.DoNotCompressArtifacts = true

	return config_obj
}

func (self *TestSuite) SetupTest() {
	if self.ConfigObj == nil {
		self.ConfigObj = self.LoadConfig()
	}

	self.LoadArtifacts(definitions)

	// Start essential services.
	self.Ctx, self.cancel = context.WithTimeout(context.Background(), time.Second*60)
	self.Sm = services.NewServiceManager(self.Ctx, self.ConfigObj)
	self.Wg = &sync.WaitGroup{}

	err := orgs.StartTestOrgManager(
		self.Ctx, self.Wg, self.ConfigObj, self.Services)
	require.NoError(self.T(), err)

	// Wait here until the indexer is all ready.
	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return indexer.(*indexing.Indexer).IsReady()
	})
}

// Parse the definitions and add them to the config so they will be
// loaded by the repository manager.
func (self *TestSuite) LoadArtifacts(definitions []string) {
	if self.ConfigObj.Autoexec == nil {
		self.ConfigObj.Autoexec = &config_proto.AutoExecConfig{}
	}

	existing_artifacts := make(map[string]*artifacts_proto.Artifact)
	for _, def := range self.ConfigObj.Autoexec.ArtifactDefinitions {
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

	self.ConfigObj.Autoexec.ArtifactDefinitions = artifacts
}

func (self *TestSuite) LoadArtifactFiles(paths ...string) {
	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	global_repo, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	for _, p := range paths {
		fd, err := os.Open(p)
		assert.NoError(self.T(), err)

		def, err := ioutil.ReadAll(fd)
		assert.NoError(self.T(), err)

		_, err = global_repo.LoadYaml(string(def), true, true)
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
