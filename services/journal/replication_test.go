package journal_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	mock_proto "www.velocidex.com/golang/velociraptor/api/mock"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/frontend"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type MockFrontendService struct {
	mock *mock_proto.MockAPIClient
}

func (self MockFrontendService) GetMinionCount() int {
	return 1
}

func (self MockFrontendService) SetGlobalMessage(
	message *api_proto.GlobalUserMessage) {
}

func (self MockFrontendService) GetGlobalMessages() []*api_proto.GlobalUserMessage {
	return nil
}

func (self MockFrontendService) GetPublicUrl(
	config_obj *config_proto.Config) (res *url.URL, err error) {
	return frontend.GetPublicUrl(config_obj)
}

func (self MockFrontendService) GetBaseURL(
	config_obj *config_proto.Config) (res *url.URL, err error) {
	return frontend.GetBaseURL(config_obj)
}

// The minion replicates to the master node.
func (self MockFrontendService) GetMasterAPIClient(ctx context.Context) (
	api_proto.APIClient, func() error, error) {
	return self.mock, func() error { return nil }, nil
}

type ReplicationTestSuite struct {
	test_utils.TestSuite

	ctrl *gomock.Controller
	mock *mock_proto.MockAPIClient
}

func (self *ReplicationTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Frontend.IsMinion = true
	self.ConfigObj.Services.FrontendServer = false
	self.ConfigObj.Services.ReplicationService = true

	self.LoadArtifactsIntoConfig([]string{`
name: Test.Artifact
type: CLIENT_EVENT
`})

	self.Services = &orgs.ServiceContainer{}
	self.Services.MockFrontendManager(&MockFrontendService{self.mock})

	self.ctrl = gomock.NewController(self.T())
	self.mock = mock_proto.NewMockAPIClient(self.ctrl)
}

func (self *ReplicationTestSuite) setupTest() {
	self.TestSuite.SetupTest()
}

func (self *ReplicationTestSuite) TestReplicationServiceStandardWatchers() {
	// The ReplicationService will call WatchEvents for both the
	// Server.Internal.Ping and Server.Internal.Notifications
	// queues.
	stream := mock_proto.NewMockAPI_WatchEventClient(self.ctrl)
	stream.EXPECT().Recv().AnyTimes().Return(nil, errors.New("Error"))

	// Record the WatchEvents calls
	var mu sync.Mutex
	watched := []string{}

	mock_watch_event_recorder := func(
		ctx context.Context, in *api_proto.EventRequest, opts ...grpc.CallOption) (
		api_proto.API_WatchEventClient, error) {
		mu.Lock()
		defer mu.Unlock()

		// only record unique listeners.
		if !utils.InString(watched, in.Queue) {
			watched = append(watched, in.Queue)
		}

		// Return an error stream - this will cause the service to
		// retry connections.
		return stream, nil
	}

	self.mock.EXPECT().WatchEvent(gomock.Any(), gomock.Any()).
		//gomock.AssignableToTypeOf(ctxInterface),
		//gomock.AssignableToTypeOf(&api_proto.EventRequest{})).
		DoAndReturn(mock_watch_event_recorder).AnyTimes()

	self.Services.MockFrontendManager(&MockFrontendService{self.mock})
	self.setupTest()

	// Wait here until we call all the watchers.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		expected := []string{
			"Server.Internal.ArtifactModification",
			"Server.Internal.MasterRegistrations",

			// The notifications service will watch for
			// notifications through us.
			"Server.Internal.Notifications",

			// Watch for ping requests from the
			// master. This is used to let the master know
			// if a client is connected to us.
			"Server.Internal.Ping",
			"Server.Internal.Pong",
		}

		for _, e := range expected {
			if !utils.InString(watched, e) {
				return false
			}
		}
		return true
	})
}

func (self *ReplicationTestSuite) TestSendingEvents() {
	self.TestReplicationServiceStandardWatchers()

	var mu sync.Mutex
	events := []*api_proto.PushEventRequest{}
	var last_error error

	// Sending some rows to an event queue
	record_push_event := func(ctx context.Context,
		in *api_proto.PushEventRequest,
		opts ...grpc.CallOption) (*emptypb.Empty, error) {
		mu.Lock()
		defer mu.Unlock()

		// On error do not capture the request
		if last_error != nil {
			return nil, last_error
		}

		events = append(events, in)
		return &emptypb.Empty{}, last_error
	}

	// Push an event into the journal service on the minion. It
	// will result in an RPC on the master to pass the event on.
	self.mock.EXPECT().PushEvents(gomock.Any(), gomock.Any()).
		DoAndReturn(record_push_event).AnyTimes()

	my_event := []*ordereddict.Dict{
		ordereddict.NewDict().Set("Foo", "Bar")}

	journal_service, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	replicator := journal_service.(*journal.ReplicationService)
	replicator.SetRetryDuration(100 * time.Millisecond)
	replicator.ProcessMasterRegistrations(ordereddict.NewDict().
		Set("Events", []interface{}{"Test.Artifact"}))

	events = nil
	err = journal_service.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		my_event, "Test.Artifact", "C.1234", "F.123")
	assert.NoError(self.T(), err)

	// Wait to see if the first event was properly delivered.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		return len(events) > 0
	})
	assert.Equal(self.T(), len(events), 1)

	// Now emulate an RPC server error.
	mu.Lock()
	last_error = errors.New("Master is down!")
	mu.Unlock()

	events = nil

	// Pushing to the journal service will transparently queue the
	// messages to a buffer file and will relay them later. NOTE:
	// This does not block, callers can not be blocked since this
	// is often on the critical path. We just dump 1000 messages
	// into the queue - this should overflow into the file.
	for i := 0; i < 1000; i++ {
		err = journal_service.PushRowsToArtifact(self.Ctx, self.ConfigObj,
			my_event, "Test.Artifact", "C.1234", "F.123")
		assert.NoError(self.T(), err)
	}

	// Wait for events to move from the channel buffer in memory to
	// the disk buffer.
	time.Sleep(time.Second)

	// Make sure we wrote something to the buffer file.
	ptr := replicator.Buffer.GetHeader().WritePointer
	assert.True(self.T(),
		ptr > 2000, fmt.Sprintf("WritePointer %v", ptr))

	// Wait a while to allow events to be delivered.
	time.Sleep(time.Second)

	// Still no event got through
	assert.Equal(self.T(), len(events), 0)

	// Now enable the server, it should just deliver all the
	// messages from the buffer file after a while as the
	// ReplicationService will retry.
	mu.Lock()
	last_error = nil
	mu.Unlock()

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		return len(events) == 1000
	})
	assert.Equal(self.T(), len(events), 1000)
}

func TestReplication(t *testing.T) {
	suite.Run(t, &ReplicationTestSuite{})
}
