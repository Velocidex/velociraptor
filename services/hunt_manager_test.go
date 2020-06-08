package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/clients"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type HuntTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	hunt_id    string
	manager    *HuntManager

	expected *flows_proto.ArtifactCollectorArgs
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

	artifacts.NextFlowIdForTests = "F.1234"

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

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.config_obj, hunt_path_manager.Path(), hunt_obj)
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

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		// The hunt index is updated.
		err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		return err == nil
	})

	time.Sleep(time.Second)

	// Check that a flow was launched.
	collection_context, err := LoadCollectionContext(self.config_obj,
		self.client_id, "F.1234")
	assert.NoError(t, err)
	assert.Equal(t, collection_context.Request.Artifacts, self.expected.Artifacts)
}

func (self *HuntTestSuite) TestHuntWithLabelClientNoLabel() {
	t := self.T()

	artifacts.NextFlowIdForTests = "F.1234"

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

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.config_obj, hunt_path_manager.Path(), hunt_obj)
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

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		// The hunt index is updated since we have seen this client
		// already (even if we decided not to launch on it).
		err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		if err != nil {
			return false
		}
		_, err := LoadCollectionContext(self.config_obj, self.client_id, "F.1234")
		return err == nil
	})
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasLabelDifferentCase() {
	t := self.T()

	artifacts.NextFlowIdForTests = "F.1234"

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

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.config_obj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	err = clients.LabelClients(self.config_obj,
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

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		// The hunt index is updated since we have seen this client
		// already (even if we decided not to launch on it).
		err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		if err != nil {
			return false
		}
		_, err := LoadCollectionContext(self.config_obj, self.client_id, "F.1234")
		return err == nil
	})

	collection_context, err := LoadCollectionContext(self.config_obj,
		self.client_id, "F.1234")
	assert.Equal(t, collection_context.Request.Artifacts, self.expected.Artifacts)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasLabel() {
	t := self.T()

	artifacts.NextFlowIdForTests = "F.1234"

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

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.config_obj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	err = clients.LabelClients(self.config_obj,
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

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		// The hunt index is updated since we have seen this client
		// already (even if we decided not to launch on it).
		err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		return err == nil
	})

	collection_context, err := LoadCollectionContext(self.config_obj,
		self.client_id, "F.1234")
	assert.NoError(t, err)
	assert.Equal(t, collection_context.Request.Artifacts, self.expected.Artifacts)
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

func LoadCollectionContext(
	config_obj *config_proto.Config,
	client_id, flow_id string) (*flows_proto.ArtifactCollectorContext, error) {

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	collection_context := &flows_proto.ArtifactCollectorContext{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	err = db.GetSubject(config_obj, flow_path_manager.Path(),
		collection_context)
	if err != nil {
		return nil, err
	}

	if collection_context.SessionId != flow_id {
		return nil, errors.New("Not found")
	}

	return collection_context, nil
}
