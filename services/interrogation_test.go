package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/clients"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type BaseServicesTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	cancel     func()
	wg         *sync.WaitGroup
	client_id  string
	flow_id    string
}

func (self *BaseServicesTestSuite) GetMemoryFileStore() *memory.MemoryFileStore {
	file_store_factory, ok := file_store.GetFileStore(
		self.config_obj).(*memory.MemoryFileStore)
	require.True(self.T(), ok)

	return file_store_factory
}

func (self *BaseServicesTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	ctx, cancel := context.WithCancel(context.Background())
	self.cancel = cancel
	self.wg = &sync.WaitGroup{}

	// Start the journaling service manually for tests.
	StartJournalService(self.config_obj)
	startInterrogationService(ctx, self.wg, self.config_obj)
	StartNotificationService(ctx, self.wg, self.config_obj)
	startVFSService(ctx, self.wg, self.config_obj)

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
}

func (self *BaseServicesTestSuite) TearDownTest() {
	// Reset the data store.
	db, err := datastore.GetDB(self.config_obj)
	require.NoError(self.T(), err)

	db.Close()
	self.cancel()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	self.wg.Wait()
}

func (self *BaseServicesTestSuite) EmulateCollection(
	artifact string, rows []*ordereddict.Dict) string {

	// Emulate a Generic.Client.Info collection: First write the
	// result set, then write the collection context.
	artifact_path_manager := result_sets.NewArtifactPathManager(
		self.config_obj, self.client_id, self.flow_id, artifact)

	// Write a result set for this artifact.
	GetJournal().PushRows(artifact_path_manager, rows)

	// Emulate a flow completion message coming from the flow processor.
	artifact_path_manager = result_sets.NewArtifactPathManager(
		self.config_obj, "server", "", "System.Flow.Completion")

	GetJournal().PushRows(artifact_path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", self.client_id).
			Set("FlowId", self.flow_id).
			Set("Flow", &flows_proto.ArtifactCollectorContext{
				ClientId:             self.client_id,
				SessionId:            self.flow_id,
				ArtifactsWithResults: []string{artifact}})})
	return self.flow_id
}

type ServicesTestSuite struct {
	BaseServicesTestSuite
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
	err = clients.LabelClients(
		self.config_obj,
		&api_proto.LabelClientsRequest{
			ClientIds: []string{self.client_id},
			Labels:    []string{"Foo"},
			Operation: "check",
		})
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

	err = GetJournal().PushRows(path_manager, []*ordereddict.Dict{
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
	suite.Run(t, &ServicesTestSuite{BaseServicesTestSuite{}})
}
