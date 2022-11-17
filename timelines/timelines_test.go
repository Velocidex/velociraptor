package timelines_test

import (
	"context"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"
)

type TimelineTestSuite struct {
	suite.Suite

	config_obj *config_proto.Config
	file_store api.FileStore
}

func (self *TimelineTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.file_store = file_store.GetFileStore(self.config_obj)
}

func (self *TimelineTestSuite) TearDownTest() {
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
}

func (self *TimelineTestSuite) TestSuperTimelineWriter() {
	path_manager := &paths.SuperTimelinePathManager{
		Name: "Test",
		Root: paths.NewNotebookPathManager("N.1234").Path(),
	}
	super, err := timelines.NewSuperTimelineWriter(self.config_obj, path_manager)
	assert.NoError(self.T(), err)

	timeline, err := super.AddChild("1")
	assert.NoError(self.T(), err)

	timeline2, err := super.AddChild("2")
	assert.NoError(self.T(), err)

	for i := int64(0); i <= 10; i++ {
		// This timeline contains evens
		timeline.Write(time.Unix(i*2, 0), ordereddict.NewDict().Set("Item", i*2))

		// This timeline contains odds
		timeline2.Write(time.Unix(i*2+1, 0), ordereddict.NewDict().Set("Item", i*2+1))
	}
	timeline.Close()
	timeline2.Close()
	super.Close()

	// test_utils.GetMemoryFileStore(self.T(), self.config_obj).Debug()
	reader, err := timelines.NewSuperTimelineReader(self.config_obj, path_manager, nil)
	assert.NoError(self.T(), err)
	defer reader.Close()

	for _, ts := range []int64{3, 4, 7} {
		reader.SeekToTime(time.Unix(ts, 0))

		ctx := context.Background()
		var last_id int64

		for item := range reader.Read(ctx) {
			value, ok := item.Row.GetInt64("Item")
			assert.True(self.T(), ok)
			assert.True(self.T(), value >= ts)

			// Items should be sequential - odds come from
			// one timeline and events from the other.
			if last_id > 0 {
				assert.Equal(self.T(), last_id+1, value)
			}
			last_id = value
		}
	}
}

func (self *TimelineTestSuite) TestTimelineWriter() {
	// Write a timeline in a notebook.
	path_manager := paths.NewNotebookPathManager("N.1234").
		SuperTimeline("T.1234").GetChild("Test")

	file_store_factory := file_store.GetFileStore(self.config_obj)
	timeline, err := timelines.NewTimelineWriter(file_store_factory, path_manager,
		utils.SyncCompleter, result_sets.TruncateMode)
	assert.NoError(self.T(), err)

	total_rows := 0
	for i := int64(0); i <= 10; i++ {
		timeline.Write(time.Unix(i*2, 0), ordereddict.NewDict().Set("Item", i*2))
		total_rows++
	}
	timeline.Close()

	// Make sure the index is correct. Each IndexRecord is 3 * 8 bytes
	// = 24 and there should be exactly one record for each row.
	index_data := test_utils.FileReadAll(self.T(), self.config_obj,
		path_manager.Index())
	assert.Equal(self.T(), len(index_data), total_rows*24)

	//test_utils.GetMemoryFileStore(self.T(), self.config_obj).Debug()

	reader, err := timelines.NewTimelineReader(file_store_factory, path_manager)
	assert.NoError(self.T(), err)
	defer reader.Close()

	ctx := context.Background()

	for _, ts := range []int64{3, 4, 7} {
		err := reader.SeekToTime(time.Unix(ts, 0))
		assert.NoError(self.T(), err)

		for row := range reader.Read(ctx) {
			value, ok := row.Row.GetInt64("Item")
			assert.True(self.T(), ok)
			assert.True(self.T(), value >= ts)
		}
	}

	// Ensure we get EOF when reading past the end of the
	// timeline. Last timestamp in the file is 20 so read time 21.
	err = reader.SeekToTime(time.Unix(21, 0))
	assert.Error(self.T(), err, "EOF")
}

func TestTimelineWriter(t *testing.T) {
	suite.Run(t, &TimelineTestSuite{})
}
