package interrogation

import (
	"context"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/labels"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type ServicesTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	flow_id    string
	sm         *services.Service
}

func (self *ServicesTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
	require.NoError(self.T(), self.sm.Start(labels.StartLabelService))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.sm.Start(inventory.StartInventoryService))
	require.NoError(self.T(), self.sm.Start(StartInterrogationService))

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
}

func (self *ServicesTestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

func (self *ServicesTestSuite) EmulateCollection(
	artifact string, rows []*ordereddict.Dict) string {

	// Emulate a Generic.Client.Info collection: First write the
	// result set, then write the collection context.
	artifact_path_manager := result_sets.NewArtifactPathManager(
		self.config_obj, self.client_id, self.flow_id, artifact)

	// Write a result set for this artifact.
	services.GetJournal().PushRows(artifact_path_manager, rows)

	// Emulate a flow completion message coming from the flow processor.
	artifact_path_manager = result_sets.NewArtifactPathManager(
		self.config_obj, "server", "", "System.Flow.Completion")

	services.GetJournal().PushRows(artifact_path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", self.client_id).
			Set("FlowId", self.flow_id).
			Set("Flow", &flows_proto.ArtifactCollectorContext{
				ClientId:             self.client_id,
				SessionId:            self.flow_id,
				ArtifactsWithResults: []string{artifact}})})
	return self.flow_id
}

func (self *ServicesTestSuite) TestInterrogationService() {
	hostname := "MyHost"
	flow_id := self.EmulateCollection(
		"Generic.Client.Info/BasicInformation", []*ordereddict.Dict{
			ordereddict.NewDict().
				Set("ClientId", self.client_id).
				Set("Hostname", hostname).
				Set("Labels", []string{"Foo"}),
		})

	// Wait here until the client is fully interrogated
	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	client_info := &actions_proto.ClientInfo{}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.config_obj, client_path_manager.Path(), client_info)
		return client_info.Hostname == hostname
	})

	// Check that we record the last flow id.
	assert.Equal(self.T(), client_info.LastInterrogateFlowId, flow_id)

	// Make sure the labels are updated in the client info
	assert.Equal(self.T(), client_info.Labels, []string{"Foo"})

	// Check the label is set on the client.
	labeler := services.GetLabeler()
	assert.True(self.T(), labeler.IsLabelSet(self.client_id, "Foo"))
	assert.NoError(self.T(), err)
}

func (self *ServicesTestSuite) TestEnrollService() {
	enroll_message := ordereddict.NewDict().Set("ClientId", self.client_id)

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	client_info := &actions_proto.ClientInfo{}
	db.GetSubject(self.config_obj, client_path_manager.Path(), client_info)

	assert.Equal(self.T(), client_info.ClientId, "")

	// Push many enroll_messages to the internal queue - this will
	// trigger the enrollment service to enrolling this client.

	// When the system is loaded it may be that multiple
	// enrollment messages are being written before the client is
	// able to be enrolled. We should always generate only a
	// single interrogate flow if the client is not known.
	path_manager := result_sets.NewArtifactPathManager(
		self.config_obj, "server" /* client_id */, "", "Server.Internal.Enrollment")

	err = services.GetJournal().PushRows(path_manager, []*ordereddict.Dict{
		enroll_message, enroll_message, enroll_message, enroll_message})
	assert.NoError(self.T(), err)

	// Wait here until the client is enrolled
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.config_obj, client_path_manager.Path(), client_info)
		return client_info.ClientId == self.client_id
	})

	// Check that a collection is scheduled.
	flow_path_manager := paths.NewFlowPathManager(self.client_id,
		client_info.LastInterrogateFlowId)
	collection_context := &flows_proto.ArtifactCollectorContext{}
	err = db.GetSubject(self.config_obj, flow_path_manager.Path(), collection_context)
	assert.Equal(self.T(), collection_context.Request.Artifacts,
		[]string{"Generic.Client.Info"})

	// Make sure only one flow is generated
	children, err := db.ListChildren(
		self.config_obj, flow_path_manager.ContainerPath(), 0, 100)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), children,
		[]string{client_info.LastInterrogateFlowId})
}

func TestInterrogationService(t *testing.T) {
	suite.Run(t, &ServicesTestSuite{})
}
