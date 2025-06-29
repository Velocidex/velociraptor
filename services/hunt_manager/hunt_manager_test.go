package hunt_manager_test

import (
	"context"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type HuntTestSuite struct {
	test_utils.TestSuite

	client_id       string
	hunt_id         string
	expected        *flows_proto.ArtifactCollectorArgs
	storage_manager launcher.FlowStorageManager
}

func (self *HuntTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.FrontendServer = true
	self.ConfigObj.Services.HuntDispatcher = true
	self.ConfigObj.Services.HuntManager = true

	self.TestSuite.SetupTest()

	self.hunt_id += "A"
	self.expected.Creator = self.hunt_id
	self.expected.FlowId = utils.CreateFlowIdFromHuntId(self.hunt_id)

	// Write a client record.
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: self.client_id,
		}})
	assert.NoError(self.T(), err)
}

func (self *HuntTestSuite) TestHuntManager() {
	t := self.T()

	// The hunt will launch the Generic.Client.Info on the client.
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_RUNNING,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
	}

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(t, err)
	hunt_dispatcher.Refresh(self.Ctx, self.ConfigObj)

	// Simulate a System.Hunt.Participation event
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id),
		},
		"System.Hunt.Participation", self.client_id, "")

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	flow_id := hunt_obj.StartRequest.FlowId
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// The hunt index is updated.
		err = indexer.CheckSimpleIndex(self.ConfigObj, paths.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		if err != nil {
			return false
		}
		_, err = self.storage_manager.LoadCollectionContext(self.Ctx,
			self.ConfigObj, self.client_id, flow_id)
		return err == nil
	})

	// Check that a flow was launched.
	collection_context, err := self.storage_manager.LoadCollectionContext(
		self.Ctx, self.ConfigObj, self.client_id, flow_id)
	assert.NoError(t, err)
	assert.Equal(t, collection_context.Request.Artifacts, self.expected.Artifacts)
}

func (self *HuntTestSuite) TestHuntWithLabelClientNoLabel() {
	t := self.T()

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

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	hunt_dispatcher.Refresh(self.Ctx, self.ConfigObj)

	// Simulate a System.Hunt.Participation event
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost"),
		},
		"System.Hunt.Participation", self.client_id, "")

	time.Sleep(time.Second)

	// No flow should be launched.
	flow_id := hunt_obj.StartRequest.FlowId
	_, err = self.storage_manager.LoadCollectionContext(self.Ctx,
		self.ConfigObj, self.client_id, flow_id)
	assert.Error(t, err)

	// Now add the label to the client. The hunt will now be
	// scheduled automatically.
	labeler := services.GetLabeler(self.ConfigObj)
	err = labeler.SetClientLabel(
		context.Background(), self.ConfigObj, self.client_id, "MyLabel")
	assert.NoError(t, err)

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// The hunt index is updated since we now run on it.
		err := indexer.CheckSimpleIndex(self.ConfigObj, paths.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		return err == nil
	})

	// The flow is now created.
	_, err = self.storage_manager.LoadCollectionContext(self.Ctx,
		self.ConfigObj, self.client_id, flow_id)
	assert.NoError(t, err)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasLabelDifferentCase() {
	t := self.T()

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

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	labeler := services.GetLabeler(self.ConfigObj)

	err = labeler.SetClientLabel(
		context.Background(), self.ConfigObj, self.client_id, "lAbEl")
	assert.NoError(t, err)

	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(t, err)
	hunt_dispatcher.Refresh(self.Ctx, self.ConfigObj)

	// Simulate a System.Hunt.Participation event
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(t, err)

	flow_id := hunt_obj.StartRequest.FlowId

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost"),
		},
		"System.Hunt.Participation", self.client_id, "")

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// The hunt index is updated since we have seen this client
		// already (even if we decided not to launch on it).
		err = indexer.CheckSimpleIndex(self.ConfigObj, paths.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		if err != nil {
			return false
		}
		_, err := self.storage_manager.LoadCollectionContext(self.Ctx,
			self.ConfigObj, self.client_id, flow_id)
		return err == nil
	})

	collection_context, err := self.storage_manager.LoadCollectionContext(
		self.Ctx, self.ConfigObj, self.client_id, flow_id)
	assert.Equal(t, collection_context.Request.Artifacts, self.expected.Artifacts)
}

func (self *HuntTestSuite) TestHuntWithOverride() {
	t := self.T()

	// Hunt is paused so normally will not receive any clients.
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_PAUSED,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
	}

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(t, err)
	hunt_dispatcher.Refresh(self.Ctx, self.ConfigObj)

	// Simulate a System.Hunt.Participation event
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Override", true),
		},
		"System.Hunt.Participation", self.client_id, "")

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	flow_id := hunt_obj.StartRequest.FlowId

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// The hunt index is updated since we have seen this client
		// already (even if we decided not to launch on it).
		err = indexer.CheckSimpleIndex(self.ConfigObj, paths.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		if err != nil {
			return false
		}

		_, err := self.storage_manager.LoadCollectionContext(self.Ctx,
			self.ConfigObj, self.client_id, flow_id)
		return err == nil
	})

	collection_context, err := self.storage_manager.LoadCollectionContext(
		self.Ctx, self.ConfigObj, self.client_id, flow_id)
	assert.NoError(t, err)
	assert.Equal(t, collection_context.Request.Artifacts, self.expected.Artifacts)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasLabel() {
	t := self.T()

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

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	labeler := services.GetLabeler(self.ConfigObj)
	err = labeler.SetClientLabel(
		context.Background(), self.ConfigObj, self.client_id, "MyLabel")
	assert.NoError(t, err)

	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(t, err)
	hunt_dispatcher.Refresh(self.Ctx, self.ConfigObj)

	// Simulate a System.Hunt.Participation event
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost"),
		},
		"System.Hunt.Participation", self.client_id, "")

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(t, err)

	flow_id := hunt_obj.StartRequest.FlowId
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// The hunt index is updated since we have seen this client
		// already (even if we decided not to launch on it).
		err = indexer.CheckSimpleIndex(self.ConfigObj, paths.HUNT_INDEX,
			self.client_id, []string{hunt_obj.HuntId})
		if err != nil {
			return false
		}

		_, err := self.storage_manager.LoadCollectionContext(
			self.Ctx, self.ConfigObj, self.client_id, flow_id)
		return err == nil
	})

	collection_context, err := self.storage_manager.LoadCollectionContext(
		self.Ctx, self.ConfigObj, self.client_id, flow_id)
	assert.NoError(t, err)
	assert.Equal(t, collection_context.Request.Artifacts, self.expected.Artifacts)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasExcludedLabel() {
	t := self.T()

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

	flow_id := hunt_obj.StartRequest.FlowId

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	labeler := services.GetLabeler(self.ConfigObj)
	err = labeler.SetClientLabel(
		context.Background(), self.ConfigObj, self.client_id, "MyLabel")
	assert.NoError(t, err)

	// Also set the excluded label - this trumps an include label.
	err = labeler.SetClientLabel(
		context.Background(), self.ConfigObj, self.client_id, "DoNotRunHunts")
	assert.NoError(t, err)

	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(t, err)
	hunt_dispatcher.Refresh(self.Ctx, self.ConfigObj)

	// Simulate a System.Hunt.Participation event
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost"),
		},
		"System.Hunt.Participation", self.client_id, "")

	time.Sleep(time.Second)

	// No flow should be launched.
	_, err = self.storage_manager.LoadCollectionContext(
		self.Ctx, self.ConfigObj, self.client_id, flow_id)
	assert.Error(t, err)
}

func (self *HuntTestSuite) TestHuntWithLabelClientHasOnlyExcludedLabel() {
	t := self.T()

	// The hunt will launch the Generic.Client.Info on the client.
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_RUNNING,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
		Condition: &api_proto.HuntCondition{
			ExcludedLabels: &api_proto.HuntLabelCondition{
				Label: []string{"DoNotRunHunts"},
			},
		},
	}

	flow_id := hunt_obj.StartRequest.FlowId

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	labeler := services.GetLabeler(self.ConfigObj)
	err = labeler.SetClientLabel(
		context.Background(), self.ConfigObj, self.client_id, "MyLabel")
	assert.NoError(t, err)

	// Also set the excluded label - this trumps an include label.
	err = labeler.SetClientLabel(
		context.Background(), self.ConfigObj, self.client_id, "DoNotRunHunts")
	assert.NoError(t, err)

	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(t, err)
	hunt_dispatcher.Refresh(self.Ctx, self.ConfigObj)

	// Simulate a System.Hunt.Participation event
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id).
			Set("Fqdn", "MyHost"),
		},
		"System.Hunt.Participation", self.client_id, "")

	time.Sleep(time.Second)

	// No flow should be launched.
	_, err = self.storage_manager.LoadCollectionContext(
		self.Ctx, self.ConfigObj, self.client_id, flow_id)
	assert.Error(t, err)
}

func (self *HuntTestSuite) TestHuntClientOSCondition() {
	t := self.T()

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
	flow_id := hunt_obj.StartRequest.FlowId

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(t, err)

	// Create a windows and linux client
	client_id_1 := "C.12321"
	client_id_2 := "C.12322"

	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: client_id_1,
			System:   "windows",
		},
	})
	assert.NoError(t, err)

	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: client_id_2,
			System:   "linux",
		},
	})
	assert.NoError(t, err)

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(t, err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(t, err)

	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(t, err)
	hunt_dispatcher.Refresh(self.Ctx, self.ConfigObj)

	// Simulate a System.Hunt.Participation event
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(t, err)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
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
		_, err = self.storage_manager.LoadCollectionContext(
			self.Ctx, self.ConfigObj, client_id_1, flow_id)
		return err == nil
	})

	// No flow should be launched on client_id_2 because it is a Linux client.
	_, err = self.storage_manager.LoadCollectionContext(
		self.Ctx, self.ConfigObj, client_id_2, flow_id)
	assert.Error(t, err)
}

// When interrogating for the first time, the initial client record
// has no OS populated so might not trigger an OS condition hunt. This
// test ensures that after interrogating the client gets another
// change to run the hunt.
func (self *HuntTestSuite) TestHuntClientOSConditionInterrogation() {
	t := self.T()

	// Create initial client with no OS set.
	self.client_id = "C.12326"

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(t, err)

	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: self.client_id,
		},
	})
	assert.NoError(t, err)

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

	acl_manager := acl_managers.NullACLManager{}
	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(t, err)

	new_hunt, err := hunt_dispatcher.CreateHunt(
		self.Ctx, self.ConfigObj, acl_manager, hunt_obj)
	assert.NoError(t, err)

	self.hunt_id = new_hunt.HuntId

	// Force the hunt manager to process a participation row
	err = hunt_manager.HuntManagerForTests.ProcessParticipationWithError(
		self.Ctx, self.ConfigObj,
		ordereddict.NewDict().
			Set("HuntId", self.hunt_id).
			Set("ClientId", self.client_id))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not match OS condition")

	// Write a new OS to it
	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: self.client_id,
			System:   "windows",
		},
	})
	assert.NoError(t, err)

	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	assert.NoError(self.T(), journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", self.client_id),
		}, "Server.Internal.Interrogation", self.client_id, ""))

	// Ensure the hunt is collected on the client.
	mdb := test_utils.GetMemoryDataStore(self.T(), self.ConfigObj)
	flow_id := hunt_obj.StartRequest.FlowId
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		task := &crypto_proto.VeloMessage{}
		path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
		err := mdb.GetSubject(self.ConfigObj,
			path_manager.Task(), task)
		return err != nil
	})
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

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(self.T(), err)

	dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(self.T(), err)
	dispatcher.Refresh(self.Ctx, self.ConfigObj)

	// Schedule a new hunt on this client if we receive a
	// participation event.
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	assert.NoError(self.T(), journal.PushRowsToArtifact(
		self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", hunt_obj.HuntId).
			Set("ClientId", self.client_id),
		}, "System.Hunt.Participation", self.client_id, ""))

	// This will schedule a hunt on this client.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		h, _ := dispatcher.GetHunt(self.Ctx, hunt_obj.HuntId)
		return h.Stats.TotalClientsScheduled == 1
	})

	// However client has not completed yet.
	h, _ := dispatcher.GetHunt(self.Ctx, hunt_obj.HuntId)
	assert.Equal(self.T(), h.Stats.TotalClientsWithResults, uint64(0))

	// For client to have completed we send a
	// System.Flow.Completion event, the hunt manager should
	// increment the total clients completed.
	flow_obj := &flows_proto.ArtifactCollectorContext{
		Request: proto.Clone(
			hunt_obj.StartRequest).(*flows_proto.ArtifactCollectorArgs),
		// No actual results but the collection is done. See #1743.
		ArtifactsWithResults: nil,
		State:                flows_proto.ArtifactCollectorContext_FINISHED,
	}

	flow_id := hunt_obj.StartRequest.FlowId
	assert.NoError(self.T(), journal.PushRowsToArtifact(
		self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("Flow", flow_obj).
			Set("FlowId", flow_id).
			Set("ClientId", self.client_id),
		}, "System.Flow.Completion", self.client_id, ""))

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		h, _ := dispatcher.GetHunt(self.Ctx, hunt_obj.HuntId)
		return h.Stats.TotalClientsWithResults == 1
	})

	// To stop the hunt, we send a hunt mutation that sets the
	// state of the hunt to stopped.
	assert.NoError(self.T(), journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", hunt_obj.HuntId).
			Set("mutation", &api_proto.HuntMutation{
				HuntId: hunt_obj.HuntId,
				State:  api_proto.Hunt_STOPPED,
			}),
		}, "Server.Internal.HuntModification", "", ""))

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		h, _ := dispatcher.GetHunt(self.Ctx, hunt_obj.HuntId)
		return h.State == api_proto.Hunt_STOPPED
	})

	h, _ = dispatcher.GetHunt(self.Ctx, hunt_obj.HuntId)
	assert.Equal(self.T(), h.State, api_proto.Hunt_STOPPED)
	assert.True(self.T(), h.Stats.Stopped)
}

// Make sure the hunt manager updates total error count
func (self *HuntTestSuite) TestHuntManagerErrors() {
	hunt_obj := &api_proto.Hunt{
		HuntId:       self.hunt_id,
		StartRequest: self.expected,
		State:        api_proto.Hunt_RUNNING,
		Stats:        &api_proto.HuntStats{},
		Expires:      uint64(time.Now().Add(7*24*time.Hour).UTC().UnixNano() / 1000),
	}

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
	err = db.SetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(self.T(), err)

	dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(self.T(), err)
	dispatcher.Refresh(self.Ctx, self.ConfigObj)

	// Schedule a new hunt on this client if we receive a
	// participation event.
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	assert.NoError(self.T(), journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("HuntId", hunt_obj.HuntId).
			Set("ClientId", self.client_id),
		}, "System.Hunt.Participation", self.client_id, ""))

	// This will schedule a hunt on this client.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		h, _ := dispatcher.GetHunt(self.Ctx, hunt_obj.HuntId)
		return h.Stats.TotalClientsScheduled == 1
	})

	// Send an error response - collection failed.
	flow_obj := &flows_proto.ArtifactCollectorContext{
		Request:              proto.Clone(hunt_obj.StartRequest).(*flows_proto.ArtifactCollectorArgs),
		ArtifactsWithResults: hunt_obj.StartRequest.Artifacts,
		State:                flows_proto.ArtifactCollectorContext_ERROR,
	}

	flow_id := hunt_obj.StartRequest.FlowId
	assert.NoError(self.T(), journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("Flow", flow_obj).
			Set("FlowId", flow_id).
			Set("ClientId", self.client_id),
		}, "System.Flow.Completion", self.client_id, ""))

	// Both TotalClientsWithResults and TotalClientsWithErrors should
	// increase.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		h, _ := dispatcher.GetHunt(self.Ctx, hunt_obj.HuntId)
		return h.Stats.TotalClientsWithResults == 1 &&
			h.Stats.TotalClientsWithErrors == 1
	})
}

func TestHuntTestSuite(t *testing.T) {
	suite.Run(t, &HuntTestSuite{
		client_id: "C.234",
		hunt_id:   "H.1",
		expected: &flows_proto.ArtifactCollectorArgs{
			Creator:   "H.1",
			ClientId:  "C.234",
			Artifacts: []string{"Generic.Client.Info"},
		},
	})
}
