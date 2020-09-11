package hunt_manager

import (
	"context"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/labels"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type HuntTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	hunt_id    string
	sm         *services.Service

	expected *flows_proto.ArtifactCollectorArgs
}

func (self *HuntTestSuite) SetupTest() {
	self.hunt_id += "A"
	self.expected.Creator = self.hunt_id

	t := self.T()
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	// Start the journaling service manually for tests.
	require.NoError(t, self.sm.Start(journal.StartJournalService))
	require.NoError(t, self.sm.Start(hunt_dispatcher.StartHuntDispatcher))
	require.NoError(t, self.sm.Start(launcher.StartLauncherService))
	require.NoError(t, self.sm.Start(labels.StartLabelService))
	require.NoError(t, self.sm.Start(notifications.StartNotificationService))
	require.NoError(t, self.sm.Start(inventory.StartInventoryService))
	require.NoError(t, self.sm.Start(repository.StartRepositoryManager))
	require.NoError(t, self.sm.Start(StartHuntManager))
}

func (self *HuntTestSuite) TearDownTest() {
	self.sm.Close()

	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
}

func (self *HuntTestSuite) TestHuntManager() {
	t := self.T()

	services.GetLauncher().SetFlowIdForTests("F.1234")

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

	services.GetHuntDispatcher().Refresh(self.config_obj)

	// Simulate a System.Hunt.Participation event
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Participate", true)},
		"System.Hunt.Participation", self.client_id, "")

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// The hunt index is updated.
		err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		if err != nil {
			return false
		}
		_, err = LoadCollectionContext(self.config_obj,
			self.client_id, "F.1234")
		return err == nil
	})

	// Check that a flow was launched.
	collection_context, err := LoadCollectionContext(self.config_obj,
		self.client_id, "F.1234")
	assert.NoError(t, err)
	assert.Equal(t, collection_context.Request.Artifacts, self.expected.Artifacts)
}

func (self *HuntTestSuite) TestHuntWithLabelClientNoLabel() {
	t := self.T()

	services.GetLauncher().SetFlowIdForTests("F.1234")

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

	services.GetHuntDispatcher().Refresh(self.config_obj)

	// Simulate a System.Hunt.Participation event
	path_manager := result_sets.NewArtifactPathManager(self.config_obj,
		self.client_id, "", "System.Hunt.Participation")
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRows(self.config_obj, path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost").
			Set("Participate", true)})

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// The hunt index is updated since we have seen this client
		// already (even if we decided not to launch on it).
		err := db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		return err == nil
	})

	// No flow should be launched.
	_, err = LoadCollectionContext(self.config_obj, self.client_id, "F.1234")
	assert.Error(t, err)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasLabelDifferentCase() {
	t := self.T()

	services.GetLauncher().SetFlowIdForTests("F.1234")

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

	labeler := services.GetLabeler()

	err = labeler.SetClientLabel(self.config_obj, self.client_id, "lAbEl")
	assert.NoError(t, err)

	services.GetHuntDispatcher().Refresh(self.config_obj)

	// Simulate a System.Hunt.Participation event
	path_manager := result_sets.NewArtifactPathManager(self.config_obj,
		self.client_id, "", "System.Hunt.Participation")
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRows(self.config_obj, path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost").
			Set("Participate", true)})

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
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

func (self *HuntTestSuite) TestHuntWithOverride() {
	t := self.T()

	services.GetLauncher().SetFlowIdForTests("F.1234")

	// Hunt is paused so normally will not receive any clients.
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_PAUSED,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
	}

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.config_obj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	services.GetHuntDispatcher().Refresh(self.config_obj)

	// Simulate a System.Hunt.Participation event
	path_manager := result_sets.NewArtifactPathManager(self.config_obj,
		self.client_id, "", "System.Hunt.Participation")
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRows(self.config_obj, path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Override", true).
			Set("Participate", true)})

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
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
	assert.NoError(t, err)
	assert.Equal(t, collection_context.Request.Artifacts, self.expected.Artifacts)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasLabel() {
	t := self.T()

	services.GetLauncher().SetFlowIdForTests("F.1234")

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

	labeler := services.GetLabeler()
	err = labeler.SetClientLabel(self.config_obj, self.client_id, "MyLabel")
	assert.NoError(t, err)

	services.GetHuntDispatcher().Refresh(self.config_obj)

	// Simulate a System.Hunt.Participation event
	path_manager := result_sets.NewArtifactPathManager(self.config_obj,
		self.client_id, "", "System.Hunt.Participation")
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRows(self.config_obj, path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost").
			Set("Participate", true)})

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
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
	assert.NoError(t, err)
	assert.Equal(t, collection_context.Request.Artifacts, self.expected.Artifacts)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasExcludedLabel() {
	t := self.T()

	services.GetLauncher().SetFlowIdForTests("F.1234")

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
			// Exclude all clients belonging to this label.
			ExcludedLabels: &api_proto.HuntLabelCondition{
				Label: []string{"DoNotRunHunts"},
			},
		},
	}

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.config_obj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	labeler := services.GetLabeler()
	err = labeler.SetClientLabel(self.config_obj, self.client_id, "MyLabel")
	assert.NoError(t, err)

	// Also set the excluded label - this trumps an include label.
	err = labeler.SetClientLabel(self.config_obj, self.client_id, "DoNotRunHunts")
	assert.NoError(t, err)

	services.GetHuntDispatcher().Refresh(self.config_obj)

	// Simulate a System.Hunt.Participation event
	path_manager := result_sets.NewArtifactPathManager(self.config_obj,
		self.client_id, "", "System.Hunt.Participation")
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRows(self.config_obj, path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost").
			Set("Participate", true)})

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// The hunt index is updated since we have seen this client
		// already (even if we decided not to launch on it).
		err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		return err == nil
	})

	// No flow should be launched.
	_, err = LoadCollectionContext(self.config_obj, self.client_id, "F.1234")
	assert.Error(t, err)
}

func TestHuntTestSuite(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "Test"

	suite.Run(t, &HuntTestSuite{
		config_obj: config_obj,
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
