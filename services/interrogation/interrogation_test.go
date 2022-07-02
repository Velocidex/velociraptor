package interrogation_test

import (
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type ServicesTestSuite struct {
	test_utils.TestSuite
	client_id string
	flow_id   string
}

func (self *ServicesTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Frontend.ServerServices.Interrogation = true

	self.LoadArtifacts([]string{`
name: Server.Internal.Enrollment
type: INTERNAL
`,
	})

	self.TestSuite.SetupTest()

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
}

func (self *ServicesTestSuite) EmulateCollection(
	artifact string, rows []*ordereddict.Dict) string {

	// Emulate a Generic.Client.Info collection: First write the
	// result set, then write the collection context.
	// Write a result set for this artifact.
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	journal.PushRowsToArtifact(self.ConfigObj,
		rows, artifact, self.client_id, self.flow_id)

	// Emulate a flow completion message coming from the flow processor.
	journal.PushRowsToArtifact(self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", self.client_id).
			Set("FlowId", self.flow_id).
			Set("Flow", &flows_proto.ArtifactCollectorContext{
				ClientId:             self.client_id,
				SessionId:            self.flow_id,
				ArtifactsWithResults: []string{artifact}})},
		"System.Flow.Completion", "server", "",
	)
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
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	client_info := &actions_proto.ClientInfo{}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
		return client_info.Hostname == hostname
	})

	// Check that we record the last flow id.
	assert.Equal(self.T(), client_info.LastInterrogateFlowId, flow_id)

	// Make sure the labels are updated in the client info
	assert.Equal(self.T(), client_info.Labels, []string{"Foo"})

	// Check the label is set on the client.
	labeler := services.GetLabeler(self.ConfigObj)
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		return labeler.IsLabelSet(self.ConfigObj, self.client_id, "Foo")
	})
	assert.NoError(self.T(), err)
}

func (self *ServicesTestSuite) TestEnrollService() {
	enroll_message := ordereddict.NewDict().Set("ClientId", self.client_id)

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	client_info := &actions_proto.ClientInfo{}
	db.GetSubject(self.ConfigObj, client_path_manager.Path(), client_info)

	assert.Equal(self.T(), client_info.ClientId, "")

	// Push many enroll_messages to the internal queue - this will
	// trigger the enrollment service to enrolling this client.

	// When the system is loaded it may be that multiple
	// enrollment messages are being written before the client is
	// able to be enrolled. We should always generate only a
	// single interrogate flow if the client is not known.
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = journal.PushRowsToArtifact(self.ConfigObj,
		[]*ordereddict.Dict{
			enroll_message, enroll_message, enroll_message, enroll_message,
		},
		"Server.Internal.Enrollment",
		"server", "")
	assert.NoError(self.T(), err)

	// Wait here until the client is enrolled
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
		return client_info.ClientId == self.client_id
	})

	// Check that a collection is scheduled.
	flow_path_manager := paths.NewFlowPathManager(self.client_id,
		client_info.LastInterrogateFlowId)
	collection_context := &flows_proto.ArtifactCollectorContext{}
	err = db.GetSubject(self.ConfigObj, flow_path_manager.Path(), collection_context)
	assert.Equal(self.T(), collection_context.Request.Artifacts,
		[]string{"Generic.Client.Info"})

	// Make sure only one flow is generated
	all_children, err := db.ListChildren(
		self.ConfigObj, flow_path_manager.ContainerPath())
	assert.NoError(self.T(), err)

	children := []api.DSPathSpec{}
	for _, c := range all_children {
		if !c.IsDir() {
			children = append(children, c)
		}
	}

	assert.Equal(self.T(), len(children), 1)
	assert.Equal(self.T(), children[0].Base(),
		client_info.LastInterrogateFlowId)
}

func TestInterrogationService(t *testing.T) {
	suite.Run(t, &ServicesTestSuite{})
}
