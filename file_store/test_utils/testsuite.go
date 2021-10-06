package test_utils

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/labels"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
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
type: CLIENT_EVENT
`, `
name: Generic.Client.Info
type: CLIENT
`,
	}
)

type TestSuite struct {
	suite.Suite
	ConfigObj *config_proto.Config
	Ctx       context.Context
	cancel    func()
	Sm        *services.Service
}

func (self *TestSuite) SetupTest() {
	var err error
	os.Setenv("VELOCIRAPTOR_CONFIG", SERVER_CONFIG)

	self.ConfigObj, err = new(config.Loader).
		WithEnvLiteralLoader("VELOCIRAPTOR_CONFIG").WithRequiredFrontend().
		WithWriteback().WithVerbose(true).
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.ConfigObj.Frontend.DoNotCompressArtifacts = true

	// Start essential services.
	self.Ctx, self.cancel = context.WithTimeout(context.Background(), time.Second*60)
	self.Sm = services.NewServiceManager(self.Ctx, self.ConfigObj)

	require.NoError(self.T(), self.Sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.Sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.Sm.Start(inventory.StartInventoryService))
	require.NoError(self.T(), self.Sm.Start(client_info.StartClientInfoService))
	require.NoError(self.T(), self.Sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.Sm.Start(repository.StartRepositoryManagerForTest))
	require.NoError(self.T(), self.Sm.Start(labels.StartLabelService))

	self.LoadArtifacts(definitions)
}

func (self *TestSuite) LoadArtifacts(definitions []string) {
	manager, _ := services.GetRepositoryManager()
	global_repo, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	for _, def := range definitions {
		_, err := global_repo.LoadYaml(def, true)
		assert.NoError(self.T(), err)
	}
}

func (self *TestSuite) LoadArtifactFiles(paths ...string) {
	manager, _ := services.GetRepositoryManager()
	global_repo, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	for _, p := range paths {
		fd, err := os.Open(p)
		assert.NoError(self.T(), err)

		def, err := ioutil.ReadAll(fd)
		assert.NoError(self.T(), err)

		_, err = global_repo.LoadYaml(string(def), true)
		assert.NoError(self.T(), err)
	}
}

func (self *TestSuite) TearDownTest() {
	self.cancel()
	self.Sm.Close()
	GetMemoryFileStore(self.T(), self.ConfigObj).Clear()
	GetMemoryDataStore(self.T(), self.ConfigObj).Clear()
}
