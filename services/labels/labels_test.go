package labels

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type LabelsTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	flow_id    string
	sm         *services.Service
}

func (self *LabelsTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	// Start the journaling service manually for tests.
	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(services.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(StartLabelService))

	self.client_id = "C.12312"
}

func (self *LabelsTestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

// Check that labels are properly populated from the index.
func (self *LabelsTestSuite) TestPopulateFromIndex() {
	db, err := datastore.GetDB(self.config_obj)
	require.NoError(self.T(), err)

	err = db.SetIndex(self.config_obj, constants.CLIENT_INDEX_URN,
		"label:Label1", []string{self.client_id})
	require.NoError(self.T(), err)

	labeler := services.GetLabeler()
	labels := labeler.GetClientLabels(self.client_id)

	require.Equal(self.T(), labels, []string{"Label1"})

	last_change_ts := labeler.LastLabelTimestamp(self.client_id)

	assert.True(self.T(), last_change_ts > 0)

	// Make sure the new record is created.
	record := &api_proto.ClientLabels{}
	client_path_manager := paths.NewClientPathManager(self.client_id)
	err = db.GetSubject(self.config_obj,
		client_path_manager.Labels(), record)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), record.Timestamp, last_change_ts)
}

func (self *LabelsTestSuite) TestAddLabel() {
	db, err := datastore.GetDB(self.config_obj)
	require.NoError(self.T(), err)

	now := uint64(time.Now().UnixNano())

	labeler := services.GetLabeler()
	err = labeler.SetClientLabel(self.client_id, "Label1")
	assert.NoError(self.T(), err)

	// Make sure the new record is created in the data store.
	record := &api_proto.ClientLabels{}
	client_path_manager := paths.NewClientPathManager(self.client_id)
	err = db.GetSubject(self.config_obj,
		client_path_manager.Labels(), record)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), record.Label, []string{"Label1"})

	// Checking against the label should work
	assert.True(self.T(), labeler.IsLabelSet(self.client_id, "Label1"))
	// case insensitive.
	assert.True(self.T(), labeler.IsLabelSet(self.client_id, "label1"))

	assert.False(self.T(), labeler.IsLabelSet(self.client_id, "Label2"))

	// The timestamp should be reasonable
	assert.True(self.T(), labeler.LastLabelTimestamp(self.client_id) > now)

	// remember the time of the last update
	now = labeler.LastLabelTimestamp(self.client_id)

	// Now remove the label.
	err = labeler.RemoveClientLabel(self.client_id, "Label1")
	assert.NoError(self.T(), err)

	assert.False(self.T(), labeler.IsLabelSet(self.client_id, "Label1"))

	// The timestamp should be later than the previous time
	assert.True(self.T(), labeler.LastLabelTimestamp(self.client_id) > now)
}

// Check that two labelers can syncronize changes between them via the
// journal.
func (self *LabelsTestSuite) TestSyncronization() {
	labeler1 := services.GetLabeler()

	// Make a second labeler to emulate a disjointed labeler from
	// another frontend.
	self.sm.Start(StartLabelService)

	labeler2 := services.GetLabeler()

	assert.NotEqual(self.T(), fmt.Sprintf("%v", labeler1),
		fmt.Sprintf("%v", labeler2))

	// Label is not set - fill the internal caches.
	assert.False(self.T(), labeler1.IsLabelSet(self.client_id, "Label1"))
	assert.False(self.T(), labeler2.IsLabelSet(self.client_id, "Label1"))

	// Set the label in one labeler and wait for the change to be
	// propagagted to the second labeler.
	err := labeler1.SetClientLabel(self.client_id, "Label1")
	assert.NoError(self.T(), err)

	assert.True(self.T(), labeler1.IsLabelSet(self.client_id, "Label1"))

	// Labeler2 should be able to pick up the changes by itself
	// within a short time.
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		return labeler2.IsLabelSet(self.client_id, "Label1")
	})
}

func TestLabelService(t *testing.T) {
	suite.Run(t, &LabelsTestSuite{})
}
