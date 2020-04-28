package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	api_mock "www.velocidex.com/golang/velociraptor/api/mock"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/clients"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/result_sets"
)

type HuntTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	hunt_id    string
	manager    *HuntManager

	expected *flows_proto.ArtifactCollectorArgs
}

type MockAPIClientFactory struct {
	mock api_proto.APIClient
}

func (self MockAPIClientFactory) GetAPIClient(
	ctx context.Context,
	config_obj *config_proto.Config) (api_proto.APIClient, func() error, error) {
	return self.mock, func() error { return nil }, nil

}

func (self *HuntTestSuite) SetupTest() {
	self.hunt_id += "A"
	self.expected.Creator = self.hunt_id
}

func (self *HuntTestSuite) TearDownTest() {
	// Reset the data store.
	db, err := datastore.GetDB(self.config_obj)
	require.NoError(self.T(), err)

	db.Close()

	self.GetMemoryFileStore().Clear()
}

func (self *HuntTestSuite) GetMemoryFileStore() *memory.MemoryFileStore {
	file_store_factory, ok := file_store.GetFileStore(
		self.config_obj).(*memory.MemoryFileStore)
	require.True(self.T(), ok)

	return file_store_factory
}

func (self *HuntTestSuite) TestHuntManager() {
	t := self.T()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Launching the hunt on the client will result in client
	// notification for that client only.
	mock := api_mock.NewMockAPIClient(ctrl)
	mock.EXPECT().CollectArtifact(
		gomock.Any(),
		self.expected,
	).Return(&flows_proto.ArtifactCollectorResponse{
		FlowId: "F.1234",
	}, nil)

	self.manager.APIClientFactory = MockAPIClientFactory{
		mock: mock,
	}

	// The hunt will launch the Generic.Client.Info on the client.
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_RUNNING,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
	}

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(t, err)

	err = db.SetSubject(self.config_obj,
		constants.GetHuntURN(hunt_obj.HuntId), hunt_obj)
	assert.NoError(t, err)

	GetHuntDispatcher().Refresh()

	// Simulate a System.Hunt.Participation event
	path_manager := result_sets.NewArtifactPathManager(self.config_obj,
		self.client_id, "", "System.Hunt.Participation")
	GetJournal().PushRows(path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost").
			Set("Participate", true)})

	// Make sure manager reacted.
	time.Sleep(1 * time.Second)

	// The hunt index is updated.
	err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
		self.client_id, []string{hunt_obj.HuntId})
	assert.NoError(t, err)
}

func (self *HuntTestSuite) TestHuntWithLabelClientNoLabel() {
	t := self.T()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := api_mock.NewMockAPIClient(ctrl)

	// Client has no label so we expect it not to start a
	// collection.
	self.manager.APIClientFactory = MockAPIClientFactory{
		mock: mock,
	}

	// The hunt will launch the Generic.Client.Info on the client.
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_RUNNING,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
		Condition: &api_proto.HuntCondition{
			UnionField: &api_proto.HuntCondition_Labels{
				Labels: &api_proto.HuntLabelCondition{
					Label: []string{"MyLabel"},
				},
			},
		},
	}

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(t, err)

	err = db.SetSubject(self.config_obj,
		constants.GetHuntURN(hunt_obj.HuntId), hunt_obj)
	assert.NoError(t, err)

	GetHuntDispatcher().Refresh()

	// Simulate a System.Hunt.Participation event
	path_manager := result_sets.NewArtifactPathManager(self.config_obj,
		self.client_id, "", "System.Hunt.Participation")
	GetJournal().PushRows(path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost").
			Set("Participate", true)})

	// Make sure manager reacted.
	time.Sleep(1 * time.Second)

	// The hunt index is updated since we have seen this client
	// already (even if we decided not to launch on it).
	err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
		self.client_id, []string{hunt_obj.HuntId})
	assert.NoError(t, err)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasLabelDifferentCase() {
	t := self.T()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := api_mock.NewMockAPIClient(ctrl)

	// Client has the label despite the case being different so we
	// expect the hunt manager to start a collection.
	mock.EXPECT().CollectArtifact(
		gomock.Any(),
		self.expected,
	).Return(&flows_proto.ArtifactCollectorResponse{
		FlowId: "F.1234",
	}, nil)

	self.manager.APIClientFactory = MockAPIClientFactory{
		mock: mock,
	}

	// The hunt will launch the Generic.Client.Info on the client.
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_RUNNING,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
		Condition: &api_proto.HuntCondition{
			UnionField: &api_proto.HuntCondition_Labels{
				Labels: &api_proto.HuntLabelCondition{
					Label: []string{"LABEL"}, // Upper case condition
				},
			},
		},
	}

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(t, err)

	err = db.SetSubject(self.config_obj,
		constants.GetHuntURN(hunt_obj.HuntId), hunt_obj)
	assert.NoError(t, err)

	_, err = clients.LabelClients(self.config_obj,
		&api_proto.LabelClientsRequest{
			ClientIds: []string{self.client_id},
			Labels:    []string{"lAbEl"}, // Lowercase label
			Operation: "set",
		})
	assert.NoError(t, err)

	GetHuntDispatcher().Refresh()

	// Simulate a System.Hunt.Participation event
	path_manager := result_sets.NewArtifactPathManager(self.config_obj,
		self.client_id, "", "System.Hunt.Participation")
	GetJournal().PushRows(path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost").
			Set("Participate", true)})

	// Make sure manager reacted.
	time.Sleep(1 * time.Second)

	// The hunt index is updated since we have seen this client
	// already (even if we decided not to launch on it).
	err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
		self.client_id, []string{hunt_obj.HuntId})
	assert.NoError(t, err)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasLabel() {
	t := self.T()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := api_mock.NewMockAPIClient(ctrl)

	// Client has the correct label so we expect the hunt manager
	// to start a collection.
	mock.EXPECT().CollectArtifact(
		gomock.Any(),
		self.expected,
	).Return(&flows_proto.ArtifactCollectorResponse{
		FlowId: "F.1234",
	}, nil)

	self.manager.APIClientFactory = MockAPIClientFactory{
		mock: mock,
	}

	// The hunt will launch the Generic.Client.Info on the client.
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_RUNNING,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
		Condition: &api_proto.HuntCondition{
			UnionField: &api_proto.HuntCondition_Labels{
				Labels: &api_proto.HuntLabelCondition{
					Label: []string{"MyLabel"},
				},
			},
		},
	}

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(t, err)

	err = db.SetSubject(self.config_obj,
		constants.GetHuntURN(hunt_obj.HuntId), hunt_obj)
	assert.NoError(t, err)

	_, err = clients.LabelClients(self.config_obj,
		&api_proto.LabelClientsRequest{
			ClientIds: []string{self.client_id},
			Labels:    []string{"MyLabel"},
			Operation: "set",
		})
	assert.NoError(t, err)

	GetHuntDispatcher().Refresh()

	// Simulate a System.Hunt.Participation event
	path_manager := result_sets.NewArtifactPathManager(self.config_obj,
		self.client_id, "", "System.Hunt.Participation")
	GetJournal().PushRows(path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost").
			Set("Participate", true)})

	// Make sure manager reacted.
	time.Sleep(1 * time.Second)

	// The hunt index is updated since we have seen this client
	// already (even if we decided not to launch on it).
	err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
		self.client_id, []string{hunt_obj.HuntId})
	assert.NoError(t, err)
}

func TestHuntTestSuite(t *testing.T) {

	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "Test"

	// Start the journaling service manually for tests.
	StartJournalService(config_obj)

	ctx := context.Background()
	wg := &sync.WaitGroup{}
	_, err := StartHuntDispatcher(ctx, wg, config_obj)
	require.NoError(t, err)

	manager, err := startHuntManager(ctx, wg, config_obj)
	require.NoError(t, err)

	suite.Run(t, &HuntTestSuite{
		config_obj: config_obj,
		manager:    manager,
		client_id:  "C.234",
		hunt_id:    "H.1",
		expected: &flows_proto.ArtifactCollectorArgs{
			Creator:   "H.1",
			ClientId:  "C.234",
			Artifacts: []string{"Generic.Client.Info"},
		},
	})
}
