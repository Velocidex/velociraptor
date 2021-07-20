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
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/services/frontend"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/labels"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
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

	self.config_obj.Frontend.IsMaster = true

	// Start the journaling service manually for tests.
	require.NoError(t, self.sm.Start(frontend.StartFrontendService))
	require.NoError(t, self.sm.Start(journal.StartJournalService))
	require.NoError(t, self.sm.Start(hunt_dispatcher.StartHuntDispatcher))
	require.NoError(t, self.sm.Start(launcher.StartLauncherService))
	require.NoError(t, self.sm.Start(labels.StartLabelService))
	require.NoError(t, self.sm.Start(notifications.StartNotificationService))
	require.NoError(t, self.sm.Start(inventory.StartInventoryService))
	require.NoError(t, self.sm.Start(repository.StartRepositoryManager))
	require.NoError(t, self.sm.Start(client_info.StartClientInfoService))
	require.NoError(t, self.sm.Start(StartHuntManager))

	// Write a client record.
	client_info_obj := &actions_proto.ClientInfo{
		ClientId: self.client_id,
	}
	client_path_manager := paths.NewClientPathManager(self.client_id)
	db, _ := datastore.GetDB(self.config_obj)
	err := db.SetSubject(self.config_obj,
		client_path_manager.Path(), client_info_obj)
	assert.NoError(self.T(), err)
}

func (self *HuntTestSuite) TearDownTest() {
	self.sm.Close()

	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
}

func (self *HuntTestSuite) TestHuntManager() {
	t := self.T()

	launcher, err := services.GetLauncher()
	assert.NoError(t, err)

	launcher.SetFlowIdForTests("F.1234")

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
			Set("ClientId", self.client_id),
		},
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

	launcher, err := services.GetLauncher()
	assert.NoError(t, err)

	launcher.SetFlowIdForTests("F.1234")

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
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost"),
		},
		"System.Hunt.Participation", self.client_id, "")

	time.Sleep(time.Second)

	// No flow should be launched.
	_, err = LoadCollectionContext(self.config_obj, self.client_id, "F.1234")
	assert.Error(t, err)

	// Now add the label to the client. The hunt will now be
	// scheduled automatically.
	labeler := services.GetLabeler()
	err = labeler.SetClientLabel(self.config_obj, self.client_id, "MyLabel")
	assert.NoError(t, err)

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// The hunt index is updated since we now run on it.
		err := db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		return err == nil
	})

	// The flow is now created.
	_, err = LoadCollectionContext(self.config_obj, self.client_id, "F.1234")
	assert.NoError(t, err)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasLabelDifferentCase() {
	t := self.T()
	launcher, err := services.GetLauncher()
	assert.NoError(t, err)

	launcher.SetFlowIdForTests("F.1234")

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
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost"),
		},
		"System.Hunt.Participation", self.client_id, "")

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

	launcher, err := services.GetLauncher()
	assert.NoError(t, err)

	launcher.SetFlowIdForTests("F.1234")

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
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Override", true),
		},
		"System.Hunt.Participation", self.client_id, "")

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

	launcher, err := services.GetLauncher()
	assert.NoError(t, err)

	launcher.SetFlowIdForTests("F.1234")

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
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost"),
		},
		"System.Hunt.Participation", self.client_id, "")

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

	launcher, err := services.GetLauncher()
	assert.NoError(t, err)

	launcher.SetFlowIdForTests("F.1234")

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
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost"),
		},
		"System.Hunt.Participation", self.client_id, "")

	time.Sleep(time.Second)

	// No flow should be launched.
	_, err = LoadCollectionContext(self.config_obj, self.client_id, "F.1234")
	assert.Error(t, err)
}

func (self *HuntTestSuite) TestHuntClientOSCondition() {
	t := self.T()

	launcher, err := services.GetLauncher()
	assert.NoError(t, err)

	launcher.SetFlowIdForTests("F.1234")

	// The hunt will launch the Generic.Client.Info on the client.
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_RUNNING,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
		Condition: &api_proto.HuntCondition{
			UnionField: &api_proto.HuntCondition_Os{
				Os: &api_proto.HuntOsCondition{
					Os: api_proto.HuntOsCondition_WINDOWS,
				},
			},
		},
	}

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(t, err)

	// Create a windows and linux client
	client_id_1 := "C.12321"
	client_id_2 := "C.12322"

	client_path_manager := paths.NewClientPathManager(client_id_1)
	err = db.SetSubject(self.config_obj,
		client_path_manager.Path(), &actions_proto.ClientInfo{
			System: "windows",
		})
	assert.NoError(t, err)

	client_path_manager = paths.NewClientPathManager(client_id_2)
	err = db.SetSubject(self.config_obj,
		client_path_manager.Path(), &actions_proto.ClientInfo{
			System: "linux",
		})
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.config_obj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	services.GetHuntDispatcher().Refresh(self.config_obj)

	// Simulate a System.Hunt.Participation event
	journal, err := services.GetJournal()
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("HuntId", self.hunt_id).
				Set("ClientId", client_id_1).
				Set("Fqdn", "MyHost1"),
			ordereddict.NewDict().
				Set("HuntId", self.hunt_id).
				Set("ClientId", client_id_2).
				Set("Fqdn", "MyHost2"),
		},
		"System.Hunt.Participation", self.client_id, "")

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// Flow should be launched on client id because it is a Windows client.
		_, err = LoadCollectionContext(self.config_obj, client_id_1, "F.1234")
		return err == nil
	})

	// No flow should be launched on client_id_2 because it is a Linux client.
	_, err = LoadCollectionContext(self.config_obj, client_id_2, "F.1234")
	assert.Error(t, err)
}

// Hunt stats are only updated by the hunt manager by sending the
// manager mutations.
func (self *HuntTestSuite) TestHuntManagerMutations() {
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_RUNNING,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
	}

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.config_obj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(self.T(), err)

	dispatcher := services.GetHuntDispatcher()
	dispatcher.Refresh(self.config_obj)

	// Schedule a new hunt on this client if we receive a
	// participation event.
	journal, err := services.GetJournal()
	assert.NoError(self.T(), err)

	assert.NoError(self.T(), journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", hunt_obj.HuntId).
			Set("ClientId", self.client_id),
		}, "System.Hunt.Participation", self.client_id, ""))

	// This will schedule a hunt on this client.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		h, _ := dispatcher.GetHunt(hunt_obj.HuntId)
		return h.Stats.TotalClientsScheduled == 1
	})

	// However client has not completed yet.
	h, _ := dispatcher.GetHunt(hunt_obj.HuntId)
	assert.Equal(self.T(), h.Stats.TotalClientsWithResults, uint64(0))

	// For client to have completed we send a
	// System.Flow.Completion event, the hunt manager should
	// increment the total clients completed.
	flow_obj := &flows_proto.ArtifactCollectorContext{
		Request:              proto.Clone(hunt_obj.StartRequest).(*flows_proto.ArtifactCollectorArgs),
		ArtifactsWithResults: hunt_obj.StartRequest.Artifacts,
		State:                flows_proto.ArtifactCollectorContext_FINISHED,
	}

	assert.NoError(self.T(), journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("Flow", flow_obj).
			Set("ClientId", self.client_id),
		}, "System.Flow.Completion", self.client_id, ""))

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		h, _ := dispatcher.GetHunt(hunt_obj.HuntId)
		return h.Stats.TotalClientsWithResults == 1
	})

	// To stop the hunt, we send a hunt mutation that sets the
	// state of the hunt to stopped.
	assert.NoError(self.T(), journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", hunt_obj.HuntId).
			Set("mutation", &api_proto.HuntMutation{
				HuntId: hunt_obj.HuntId,
				State:  api_proto.Hunt_STOPPED,
			}),
		}, "Server.Internal.HuntModification", "", ""))

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		h, _ := dispatcher.GetHunt(hunt_obj.HuntId)
		return h.State == api_proto.Hunt_STOPPED
	})

	h, _ = dispatcher.GetHunt(hunt_obj.HuntId)
	assert.Equal(self.T(), h.State, api_proto.Hunt_STOPPED)
	assert.True(self.T(), h.Stats.Stopped)
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
