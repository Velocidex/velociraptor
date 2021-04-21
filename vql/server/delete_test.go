package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
)

type DeleteTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	sm         *services.Service

	client_id, flow_id string
}

func (self *DeleteTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().WithVerbose(true).
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.config_obj.Frontend.DoNotCompressArtifacts = true

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(inventory.StartInventoryService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))

	self.client_id = "C.123"
	self.flow_id = "F.123"
}

func (self *DeleteTestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

func (self *DeleteTestSuite) TestDeleteClient() {

}

func TestDeletePlugin(t *testing.T) {
	suite.Run(t, &DeleteTestSuite{})
}
