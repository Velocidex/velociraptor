package journal_test

import (
	"context"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/emptypb"
	mock_proto "www.velocidex.com/golang/velociraptor/api/mock"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type MockFrontendService struct {
	mock *mock_proto.MockAPIClient
}

func (self MockFrontendService) IsMaster() bool {
	return false
}

// The minion replicates to the master node.
func (self MockFrontendService) GetMasterAPIClient(ctx context.Context) (
	api_proto.APIClient, func() error, error) {
	return self.mock, func() error { return nil }, nil
}

type ReplicationTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	sm         *services.Service
	ctrl       *gomock.Controller
	mock       *mock_proto.MockAPIClient
}

func (self *ReplicationTestSuite) startServices() {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	t := self.T()
	assert.NoError(t, self.sm.Start(journal.StartJournalService))
	assert.NoError(t, self.sm.Start(notifications.StartNotificationService))
	assert.NoError(t, self.sm.Start(inventory.StartInventoryService))
	assert.NoError(t, self.sm.Start(launcher.StartLauncherService))
	assert.NoError(t, self.sm.Start(repository.StartRepositoryManagerForTest))

	// Set retry to be faster.
	journal_service, err := services.GetJournal()
	assert.NoError(self.T(), err)

	replicator := journal_service.(*journal.ReplicationService)
	replicator.RetryDuration = 100 * time.Millisecond
}

func (self *ReplicationTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.ctrl = gomock.NewController(self.T())
	self.mock = mock_proto.NewMockAPIClient(self.ctrl)

	// Replication service only runs on the minion node. We mock
	// the minion frontend manager so we can inject the RPC mock.
	services.RegisterFrontendManager(&MockFrontendService{self.mock})
}

func (self *ReplicationTestSuite) TearDownTest() {
	self.sm.Close()
	self.ctrl.Finish()

	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

func (self *ReplicationTestSuite) TestReplicationServiceStandardWatchers() {

	// The ReplicationService will call WatchEvents for both the
	// Server.Internal.Ping and Server.Internal.Notifications
	// queues.
	stream := mock_proto.NewMockAPI_WatchEventClient(self.ctrl)
	stream.EXPECT().Recv().AnyTimes().Return(nil, errors.New("Error"))

	// Record the WatchEvents calls
	watched := []string{}
	mock_watch_event_recorder := func(
		ctx context.Context, in *api_proto.EventRequest) (
		api_proto.API_WatchEventClient, error) {
		watched = append(watched, in.Queue)
		return stream, nil
	}

	self.mock.EXPECT().WatchEvent(gomock.Any(), gomock.Any()).
		//gomock.AssignableToTypeOf(ctxInterface),
		//gomock.AssignableToTypeOf(&api_proto.EventRequest{})).
		DoAndReturn(mock_watch_event_recorder).AnyTimes()

	self.startServices()

	// Wait here until we call all the watchers.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return vtesting.CompareStrings(watched, []string{
			// Watch for ping requests from the
			// master. This is used to let the master know
			// if a client is connected to us.
			"Server.Internal.Ping",

			// The notifications service will watch for
			// notifications through us.
			"Server.Internal.Notifications",
		})
	})
}

func (self *ReplicationTestSuite) TestSendingEvents() {
	self.TestReplicationServiceStandardWatchers()

	events := []*api_proto.PushEventRequest{}
	var last_error error

	// Sending some rows to an event queue
	record_push_event := func(ctx context.Context,
		in *api_proto.PushEventRequest) (*emptypb.Empty, error) {
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

	journal_service, err := services.GetJournal()
	assert.NoError(self.T(), err)

	replicator := journal_service.(*journal.ReplicationService)
	replicator.RetryDuration = 100 * time.Millisecond

	events = nil
	err = journal_service.PushRowsToArtifact(self.config_obj,
		my_event, "Test.Artifact", "C.1234", "F.123")
	assert.NoError(self.T(), err)

	// Wait to see if the first event was properly delivered.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return len(events) > 0
	})
	assert.Equal(self.T(), len(events), 1)

	// Now emulate an RPC server error.
	last_error = errors.New("Master is down!")

	events = nil

	// Pushing to the journal service will transparently queue the
	// messages to a buffer file and will relay them later. NOTE:
	// This does not block, callers can not be blocked since this
	// is often on the critical path. We just dump 1000 messages
	// into the queue - this should overflow into the file.
	for i := 0; i < 1000; i++ {
		err = journal_service.PushRowsToArtifact(self.config_obj,
			my_event, "Test.Artifact", "C.1234", "F.123")
		assert.NoError(self.T(), err)
	}

	// Make sure we wrote something to the buffer file.
	assert.True(self.T(), replicator.Buffer.Header.WritePointer > 2000)

	// Wait a while to allow events to be delivered.
	time.Sleep(time.Second)

	// Still no event got through
	assert.Equal(self.T(), len(events), 0)

	// Now enable the server, it should just deliver all the
	// messages from the buffer file after a while as the
	// ReplicationService will retry.
	last_error = nil
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return len(events) == 1000
	})
	assert.Equal(self.T(), len(events), 1000)
}

func TestReplication(t *testing.T) {
	suite.Run(t, &ReplicationTestSuite{})
}
