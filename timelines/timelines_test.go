package timelines

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
	path_manager := &SuperTimelinePathManager{"Test", "notebooks/N.1234/"}
	super, err := NewSuperTimelineWriter(self.config_obj, path_manager)
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
	reader, err := NewSuperTimelineReader(self.config_obj, path_manager, nil)
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
	path_manager := &TimelinePathManager{"T.1234", "Test"}
	file_store_factory := file_store.GetFileStore(self.config_obj)
	timeline, err := NewTimelineWriter(file_store_factory, path_manager)
	assert.NoError(self.T(), err)

	for i := int64(0); i <= 10; i++ {
		timeline.Write(time.Unix(i*2, 0), ordereddict.NewDict().Set("Item", i*2))
	}
	timeline.Close()

	//	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Debug()

	reader, err := NewTimelineReader(file_store_factory, path_manager)
	assert.NoError(self.T(), err)
	defer reader.Close()

	ctx := context.Background()

	for _, ts := range []int64{3, 4, 7} {
		reader.SeekToTime(time.Unix(ts, 0))
		for row := range reader.Read(ctx) {
			value, ok := row.Row.GetInt64("Item")
			assert.True(self.T(), ok)
			assert.True(self.T(), value >= ts)
		}
	}
}

func TestTimelineWriter(t *testing.T) {
	suite.Run(t, &TimelineTestSuite{})
}
