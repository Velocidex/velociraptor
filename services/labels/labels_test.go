package labels_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
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
	Clock     utils.Clock
}

func (self *LabelsTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	self.client_id = "C.12312"
	self.Clock = &utils.IncClock{}

	// Set an incremental clock on the labeler.
	labeler := services.GetLabeler().(*labels.Labeler)
	labeler.Clock = self.Clock
}

func (self *LabelsTestSuite) TestAddLabel() {
	db, err := datastore.GetDB(self.ConfigObj)
	require.NoError(self.T(), err)

	now := uint64(self.Clock.Now().UnixNano())

	labeler := services.GetLabeler()
	err = labeler.SetClientLabel(self.ConfigObj, self.client_id, "Label1")
	assert.NoError(self.T(), err)

	// Set the label twice - it should only set one label.
	err = labeler.SetClientLabel(self.ConfigObj, self.client_id, "Label1")
	assert.NoError(self.T(), err)

	// Make sure the new record is created in the data store.
	record := &api_proto.ClientLabels{}
	client_path_manager := paths.NewClientPathManager(self.client_id)
	err = db.GetSubject(self.ConfigObj,
		client_path_manager.Labels(), record)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), record.Label, []string{"Label1"})

	// Checking against the label should work
	assert.True(self.T(), labeler.IsLabelSet(self.ConfigObj, self.client_id, "Label1"))
	// case insensitive.
	assert.True(self.T(), labeler.IsLabelSet(self.ConfigObj, self.client_id, "label1"))

	assert.False(self.T(), labeler.IsLabelSet(self.ConfigObj, self.client_id, "Label2"))

	// All clients belong to the All label.
	assert.True(self.T(), labeler.IsLabelSet(self.ConfigObj, self.client_id, "All"))

	// The timestamp should be reasonable
	assert.Greater(self.T(), labeler.LastLabelTimestamp(
		self.ConfigObj, self.client_id), now)

	// remember the time of the last update
	now = labeler.LastLabelTimestamp(self.ConfigObj, self.client_id)

	// Now remove the label.
	err = labeler.RemoveClientLabel(self.ConfigObj, self.client_id, "Label1")
	assert.NoError(self.T(), err)

	assert.False(self.T(), labeler.IsLabelSet(self.ConfigObj, self.client_id, "Label1"))

	// The timestamp should be later than the previous time
	assert.Greater(self.T(), labeler.LastLabelTimestamp(self.ConfigObj, self.client_id), now)
}

// Check that two labelers can syncronize changes between them via the
// journal.
func (self *LabelsTestSuite) TestSyncronization() {
	labeler1 := services.GetLabeler()

	// Make a second labeler to emulate a disjointed labeler from
	// another frontend.
	self.Sm.Start(labels.StartLabelService)

	labeler2 := services.GetLabeler()

	assert.NotEqual(self.T(), fmt.Sprintf("%v", labeler1),
		fmt.Sprintf("%v", labeler2))

	// Label is not set - fill the internal caches.
	assert.False(self.T(), labeler1.IsLabelSet(self.ConfigObj, self.client_id, "Label1"))
	assert.False(self.T(), labeler2.IsLabelSet(self.ConfigObj, self.client_id, "Label1"))

	// Set the label in one labeler and wait for the change to be
	// propagagted to the second labeler.
	err := labeler1.SetClientLabel(self.ConfigObj, self.client_id, "Label1")
	assert.NoError(self.T(), err)

	assert.True(self.T(), labeler1.IsLabelSet(self.ConfigObj, self.client_id, "Label1"))

	// Labeler2 should be able to pick up the changes by itself
	// within a short time.
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		return labeler2.IsLabelSet(self.ConfigObj, self.client_id, "Label1")
	})
}

func TestLabelService(t *testing.T) {
	suite.Run(t, &LabelsTestSuite{})
}
