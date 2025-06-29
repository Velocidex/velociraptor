package interrogation_test

import (
	"fmt"
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
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/interrogation"
	"www.velocidex.com/golang/velociraptor/utils"
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
	self.ConfigObj.Services.Interrogation = true
	self.ConfigObj.Defaults.UnauthenticatedLruTimeoutSec = -1

	self.LoadArtifactsIntoConfig([]string{`
name: Server.Internal.Enrollment
type: INTERNAL
`,
	})

	self.client_id = fmt.Sprintf("C.1%d", utils.GetId())
	self.flow_id = "F.1232"

	self.TestSuite.SetupTest()

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: self.client_id,
		},
	})

	interrogation.DEBUG = true
}

func (self *ServicesTestSuite) EmulateCollection(
	artifact string, rows []*ordereddict.Dict) string {

	// Emulate a Generic.Client.Info collection: First write the
	// result set, then write the collection context.
	// Write a result set for this artifact.
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		rows, artifact, self.client_id, self.flow_id)

	// Emulate a flow completion message coming from the flow processor.
	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
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
				Set("Name", "velociraptor").
				Set("OS", "windows").
				Set("Hostname", hostname).
				Set("Labels", []string{"Foo"}),
		})

	// Wait here until the client is fully interrogated
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	var client_info *services.ClientInfo
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		client_info, err = client_info_manager.Get(self.Ctx, self.client_id)
		return err == nil && client_info.Hostname == hostname
	})

	// Check that we record the last flow id.
	assert.Equal(self.T(), client_info.LastInterrogateFlowId, flow_id)

	// Make sure the labels are updated in the client info
	assert.Equal(self.T(), client_info.Labels, []string{"Foo"})

	// Check the label is set on the client.
	labeler := services.GetLabeler(self.ConfigObj)
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return labeler.IsLabelSet(
			self.Ctx, self.ConfigObj, self.client_id, "Foo")
	})
	assert.NoError(self.T(), err)
}

func (self *ServicesTestSuite) TestEnrollService() {
	enroll_message := ordereddict.NewDict().Set("ClientId", self.client_id)

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Check the client does not exist in the datastore yet
	client_path_manager := paths.NewClientPathManager(self.client_id)
	client_info := &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{}}
	err = db.GetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
	assert.Error(self.T(), err)
	assert.Equal(self.T(), client_info.ClientId, "")

	// Push many enroll_messages to the internal queue - this will
	// trigger the enrollment service to enrolling this client.

	// When the system is loaded it may be that multiple
	// enrollment messages are being written before the client is
	// able to be enrolled. We should always generate only a
	// single interrogate flow if the client is not known.
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	ctx := self.Ctx
	err = journal.PushRowsToArtifact(ctx, self.ConfigObj,
		[]*ordereddict.Dict{
			enroll_message, enroll_message, enroll_message, enroll_message,
		},
		"Server.Internal.Enrollment",
		"server", "")
	assert.NoError(self.T(), err)

	// Wait here until the client is enrolled
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		client_info, err = client_info_manager.Get(self.Ctx, self.client_id)

		return err == nil && client_info.ClientId == self.client_id &&
			client_info.LastInterrogateFlowId != ""
	})

	// Check that a collection is scheduled.
	flow_path_manager := paths.NewFlowPathManager(self.client_id,
		client_info.LastInterrogateFlowId)
	collection_context := &flows_proto.ArtifactCollectorContext{}
	err = db.GetSubject(self.ConfigObj, flow_path_manager.Path(), collection_context)
	assert.NoError(self.T(), err)

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

	if len(children) > 1 {
		test_utils.GetMemoryDataStore(self.T(), self.ConfigObj).Dump()
		test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()
		json.Debug(children)
	}

	assert.Equal(self.T(), len(children), 1)
	assert.Equal(self.T(), children[0].Base(),
		client_info.LastInterrogateFlowId)
}

func TestInterrogationService(t *testing.T) {
	suite.Run(t, &ServicesTestSuite{})
}
