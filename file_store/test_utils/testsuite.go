package test_utils

import (
	"context"
	"os"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/labels"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
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
	require.NoError(self.T(), self.Sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.Sm.Start(repository.StartRepositoryManager))
	require.NoError(self.T(), self.Sm.Start(labels.StartLabelService))
}

func (self *TestSuite) TearDownTest() {
	self.cancel()
	self.Sm.Close()
	GetMemoryFileStore(self.T(), self.ConfigObj).Clear()
	GetMemoryDataStore(self.T(), self.ConfigObj).Clear()
}
