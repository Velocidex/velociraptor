package labels_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/labels"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type LabelsTestSuite struct {
	test_utils.TestSuite
	client_id string
	flow_id   string

	closer func()
}

func (self *LabelsTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	self.client_id = "C.12312"
	self.closer = utils.MockTime(&utils.IncClock{})

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: self.client_id,
		},
	})
}

func (self *LabelsTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	if self.closer != nil {
		self.closer()
	}
}

// Check how labels interact with the indexing service
func (self *LabelsTestSuite) TestLabelsAndIndexing() {

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Search for clients with label
	resp, err := indexer.SearchClients(self.Ctx, self.ConfigObj,
		&api_proto.SearchClientsRequest{
			Offset: 0, Limit: 10,
			Query: "label:Label1",
		}, "admin")
	assert.NoError(self.T(), err)

	// No clients have this label yet
	assert.Equal(self.T(), 0, len(resp.Items))

	labeler := services.GetLabeler(self.ConfigObj)
	err = labeler.SetClientLabel(
		self.Ctx, self.ConfigObj, self.client_id, "Label1")
	assert.NoError(self.T(), err)

	resp, err = indexer.SearchClients(self.Ctx, self.ConfigObj,
		&api_proto.SearchClientsRequest{
			Offset: 0, Limit: 10,
			Query: "label:Label1",
		}, "admin")
	assert.NoError(self.T(), err)

	// Client should have label now.
	assert.Equal(self.T(), 1, len(resp.Items))
	assert.Equal(self.T(), self.client_id, resp.Items[0].ClientId)

	// Now remove the label.
	err = labeler.RemoveClientLabel(
		self.Ctx, self.ConfigObj, self.client_id, "Label1")
	assert.NoError(self.T(), err)

	resp, err = indexer.SearchClients(self.Ctx, self.ConfigObj,
		&api_proto.SearchClientsRequest{
			Offset: 0, Limit: 10,
			Query: "label:Label1",
		}, "admin")
	assert.NoError(self.T(), err)

	// No client should not match the label
	assert.Equal(self.T(), 0, len(resp.Items))
}

func (self *LabelsTestSuite) TestAddLabel() {
	now := uint64(utils.GetTime().Now().UnixNano())

	labeler := services.GetLabeler(self.ConfigObj)
	err := labeler.SetClientLabel(
		self.Ctx, self.ConfigObj, self.client_id, "Label1")
	assert.NoError(self.T(), err)

	// Set the label twice - it should only set one label.
	err = labeler.SetClientLabel(
		self.Ctx, self.ConfigObj, self.client_id, "Label1")
	assert.NoError(self.T(), err)

	// Checking against the label should work
	assert.True(self.T(), labeler.IsLabelSet(
		self.Ctx, self.ConfigObj, self.client_id, "Label1"))
	// case insensitive.
	assert.True(self.T(), labeler.IsLabelSet(
		self.Ctx, self.ConfigObj, self.client_id, "label1"))

	assert.False(self.T(), labeler.IsLabelSet(
		self.Ctx, self.ConfigObj, self.client_id, "Label2"))

	// All clients belong to the All label.
	assert.True(self.T(), labeler.IsLabelSet(
		self.Ctx, self.ConfigObj, self.client_id, "All"))

	// The timestamp should be reasonable
	assert.Greater(self.T(), labeler.LastLabelTimestamp(
		self.Ctx, self.ConfigObj, self.client_id), now)

	// remember the time of the last update
	now = labeler.LastLabelTimestamp(
		self.Ctx, self.ConfigObj, self.client_id)

	// Now remove the label.
	err = labeler.RemoveClientLabel(
		self.Ctx, self.ConfigObj, self.client_id, "Label1")
	assert.NoError(self.T(), err)

	assert.False(self.T(), labeler.IsLabelSet(
		self.Ctx, self.ConfigObj, self.client_id, "Label1"))

	// The timestamp should be later than the previous time
	assert.Greater(self.T(), labeler.LastLabelTimestamp(
		self.Ctx, self.ConfigObj, self.client_id), now)
}

// Check that two labelers can syncronize changes between them via the
// journal.
func (self *LabelsTestSuite) TestSyncronization() {
	labeler1 := services.GetLabeler(self.ConfigObj)

	// Make a second labeler to emulate a disjointed labeler from
	// another frontend.
	labeler2, err := labels.NewLabelerService(self.Ctx, self.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	assert.NotEqual(self.T(), fmt.Sprintf("%v", labeler1),
		fmt.Sprintf("%v", labeler2))

	// Label is not set - fill the internal caches.
	assert.False(self.T(), labeler1.IsLabelSet(
		self.Ctx, self.ConfigObj, self.client_id, "Label1"))
	assert.False(self.T(), labeler2.IsLabelSet(
		self.Ctx, self.ConfigObj, self.client_id, "Label1"))

	// Set the label in one labeler and wait for the change to be
	// propagagted to the second labeler.
	err = labeler1.SetClientLabel(
		self.Ctx, self.ConfigObj, self.client_id, "Label1")
	assert.NoError(self.T(), err)

	assert.True(self.T(), labeler1.IsLabelSet(
		self.Ctx, self.ConfigObj, self.client_id, "Label1"))

	// Labeler2 should be able to pick up the changes by itself
	// within a short time.
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		return labeler2.IsLabelSet(
			self.Ctx, self.ConfigObj, self.client_id, "Label1")
	})
}

func TestLabelService(t *testing.T) {
	suite.Run(t, &LabelsTestSuite{})
}
