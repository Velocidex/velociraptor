package hunt_dispatcher_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

const (
	FORCE_REFRESH = hunt_dispatcher.FORCE_REFRESH
)

type HuntDispatcherTestSuite struct {
	test_utils.TestSuite

	hunt_id string

	master_dispatcher *hunt_dispatcher.HuntDispatcher
	minion_dispatcher *hunt_dispatcher.HuntDispatcher

	time_closer func()
}

func (self *HuntDispatcherTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.FrontendServer = true
	self.ConfigObj.Services.HuntDispatcher = true
	self.ConfigObj.Services.HuntManager = true
	self.ConfigObj.Services.RepositoryManager = true
	self.ConfigObj.Services.JournalService = true

	// Disable refresh - we will do it in the test
	self.ConfigObj.Defaults.HuntDispatcherRefreshSec = -1

	journal.PushRowsToArtifactAsyncIsSynchrnous = true
	hunt_dispatcher.DEBUG = true

	self.LoadArtifactsIntoConfig([]string{`
name: Server.Internal.HuntUpdate
type: INTERNAL
`})

	self.time_closer = utils.MockTime(&utils.IncClock{})

	// Start off with some data in the datastore.
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	for i := 0; i < 5; i++ {
		now := utils.GetTime().Now().Unix()
		hunt_obj := &api_proto.Hunt{
			HuntId:    fmt.Sprintf("H.%d", i),
			State:     api_proto.Hunt_RUNNING,
			Version:   now * 1000000,
			StartTime: uint64(now),

			// Set the expiry very far in the future
			Expires: uint64(now+1000) * 1000000,
		}
		hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
		assert.NoError(self.T(),
			db.SetSubject(self.ConfigObj,
				hunt_path_manager.Path(), hunt_obj))
	}

	self.TestSuite.SetupTest()

	// Make a master and minion dispatchers.
	master_dispatcher, err := hunt_dispatcher.NewHuntDispatcher(
		self.Ctx, self.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	self.master_dispatcher = master_dispatcher.(*hunt_dispatcher.HuntDispatcher)

	// Wait here until the dispatcher is initialized.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		hunt, pres := self.master_dispatcher.GetHunt(self.Ctx, "H.4")
		return pres && hunt.HuntId == "H.4"
	})

	// Now flush the index to disk.
	err = self.master_dispatcher.Refresh(self.Ctx, self.ConfigObj, FORCE_REFRESH)
	assert.NoError(self.T(), err)

	// Start a minion hunt dispatcher
	config_obj := proto.Clone(self.ConfigObj).(*config_proto.Config)
	config_obj.Frontend.IsMinion = true

	minion_dispatcher, err := hunt_dispatcher.NewHuntDispatcher(
		self.Ctx, self.Wg, config_obj)
	assert.NoError(self.T(), err)

	self.minion_dispatcher = minion_dispatcher.(*hunt_dispatcher.HuntDispatcher)

	// Check that hunts are loaded in both master and minion dispatchers.
	hunt, pres := self.master_dispatcher.GetHunt(self.Ctx, "H.4")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt.HuntId, "H.4")

	hunt, pres = self.minion_dispatcher.GetHunt(self.Ctx, "H.4")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt.HuntId, "H.4")
}

func (self *HuntDispatcherTestSuite) TestLoadingFromDisk() {
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// All hunts are now running.
		hunts := self.getAllHunts()
		if len(hunts) != 5 {
			return false
		}
		for _, h := range hunts {
			if h.State != api_proto.Hunt_RUNNING {
				return false
			}
		}
		return true
	})
}

func (self *HuntDispatcherTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	if self.time_closer != nil {
		self.time_closer()
	}
}

func (self *HuntDispatcherTestSuite) TestModifyingHuntFlushToDatastore() {
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Modify a hunt and flush to datastore immediately
	ctx := context.Background()
	modification := self.master_dispatcher.ModifyHuntObject(ctx, "H.1",
		func(hunt *api_proto.Hunt) services.HuntModificationAction {
			hunt.State = api_proto.Hunt_STOPPED
			return services.HuntFlushToDatastore
		})

	// Changes should be visible in the data store immediately.
	assert.Equal(self.T(), modification, services.HuntFlushToDatastore)

	hunt_path_manager := paths.NewHuntPathManager("H.1")
	hunt_obj := &api_proto.Hunt{}
	err = db.GetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_STOPPED)

	// Should also be visible in master
	hunt_obj, pres := self.master_dispatcher.GetHunt(self.Ctx, "H.1")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_STOPPED)

	// But not immediately visible in minion
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		hunt_obj, pres = self.minion_dispatcher.GetHunt(self.Ctx, "H.1")
		return pres && hunt_obj.HuntId == "H.1"
	})

	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_RUNNING)
}

func (self *HuntDispatcherTestSuite) TestIndexSerialization() {
	// Modify a hunt and flush to datastore immediately
	ctx := context.Background()
	modification := self.master_dispatcher.ModifyHuntObject(ctx, "H.1",
		func(hunt *api_proto.Hunt) services.HuntModificationAction {
			hunt.State = api_proto.Hunt_STOPPED
			return services.HuntFlushToDatastore
		})

	// Changes should be visible in the data store immediately.
	assert.Equal(self.T(), modification, services.HuntFlushToDatastore)

	// Force the master to dump the index.
	err := self.master_dispatcher.Refresh(ctx, self.ConfigObj, FORCE_REFRESH)
	assert.NoError(self.T(), err)

	// Now read the index from a new storage manager.
	storage := hunt_dispatcher.NewHuntStorageManagerImpl(self.ConfigObj)
	n, err := storage.(*hunt_dispatcher.HuntStorageManagerImpl).
		LoadHuntsFromIndex(self.Ctx, self.ConfigObj)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), n, 5)

	hunts, _, err := storage.ListHunts(self.Ctx,
		result_sets.ResultSetOptions{}, 0, 100)
	assert.NoError(self.T(), err)

	if len(hunts) != 5 {
		json.Dump(hunts)
	}

	assert.Equal(self.T(), 5, len(hunts))
	assert.Equal(self.T(), "H.4", hunts[0].HuntId)
	assert.Equal(self.T(), "H.0", hunts[4].HuntId)
}

func (self *HuntDispatcherTestSuite) TestModifyingHuntPropagateChanges() {
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Check the hunt is running in the data store.
	hunt_obj, pres := self.minion_dispatcher.GetHunt(self.Ctx, "H.2")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_RUNNING)

	hunt_obj, pres = self.master_dispatcher.GetHunt(self.Ctx, "H.2")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_RUNNING)

	// Now modify a hunt with services.HuntPropagateChanges
	ctx := self.Ctx
	modification := self.master_dispatcher.ModifyHuntObject(ctx, "H.2",
		func(hunt *api_proto.Hunt) services.HuntModificationAction {
			hunt.State = api_proto.Hunt_STOPPED
			self.master_dispatcher.Debug("Sending %v", hunt)
			return services.HuntPropagateChanges
		})

	// Changes may not visible in the data store immediately.
	assert.Equal(self.T(), modification, services.HuntPropagateChanges)

	// But they should be visible in master
	hunt_obj, pres = self.master_dispatcher.GetHunt(self.Ctx, "H.2")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_STOPPED)

	// Eventually this should be visible in the minion as well.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		hunt_obj, pres = self.minion_dispatcher.GetHunt(self.Ctx, "H.2")
		return pres && hunt_obj.State == api_proto.Hunt_STOPPED
	})

	// And eventually also be visible in the filestore
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		hunt_path_manager := paths.NewHuntPathManager("H.2")
		hunt_obj := &api_proto.Hunt{}
		err = db.GetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
		assert.NoError(self.T(), err)
		return hunt_obj.State == api_proto.Hunt_STOPPED
	})
}

func (self *HuntDispatcherTestSuite) getAllHunts() []*api_proto.Hunt {
	// Get the list of all hunts
	hunts := []*api_proto.Hunt{}
	err := self.master_dispatcher.ApplyFuncOnHunts(
		self.Ctx, services.AllHunts,
		func(hunt *api_proto.Hunt) error {
			hunts = append(hunts, hunt)
			return nil
		})
	assert.NoError(self.T(), err)

	sort.Slice(hunts, func(i, j int) bool {
		return hunts[i].HuntId < hunts[j].HuntId
	})
	return hunts
}

func (self *HuntDispatcherTestSuite) TestExpiringHunts() {
	closer := utils.MockTime(utils.RealClock{})
	defer closer()

	hunt_id := "H.121222"

	master_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(self.T(), err)

	now := utils.GetTime().Now().Unix()
	acl_manager := acl_managers.NullACLManager{}

	hunt_obj, err := master_dispatcher.CreateHunt(self.Ctx, self.ConfigObj,
		acl_manager, &api_proto.Hunt{
			HuntId:    hunt_id,
			State:     api_proto.Hunt_RUNNING,
			Version:   now,
			StartTime: uint64(now),
			Expires:   uint64(now+1) * 1000000,
			StartRequest: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"Generic.Client.Info"},
			},
		})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_RUNNING)

	// Fast forward the time
	closer = utils.MockTime(utils.RealClockWithOffset{Duration: 600 * time.Second})
	defer closer()

	// And eventually also visible in minion
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// And eventually also visible in minion
		err = master_dispatcher.Refresh(self.Ctx, self.ConfigObj, FORCE_REFRESH)
		assert.NoError(self.T(), err)

		hunt_obj, pres := master_dispatcher.GetHunt(self.Ctx, hunt_id)
		assert.True(self.T(), pres)

		return hunt_obj.State == api_proto.Hunt_STOPPED
	})

}

func (self *HuntDispatcherTestSuite) TestDeleteHunts() {
	hunt_id := "H.3"

	_, pres := self.master_dispatcher.GetHunt(self.Ctx, hunt_id)
	assert.True(self.T(), pres)

	// Delete the new hunt
	self.master_dispatcher.MutateHunt(self.Ctx, self.ConfigObj,
		&api_proto.HuntMutation{
			HuntId: hunt_id,
			State:  api_proto.Hunt_DELETED,
		})

	// Make sure the changes are written to the disk. This will happen eventually anyway.
	err := self.master_dispatcher.Refresh(self.Ctx, self.ConfigObj, FORCE_REFRESH)
	assert.NoError(self.T(), err)

	// This will happen in time but we force it now to make the test
	// go faster
	err = self.minion_dispatcher.Refresh(self.Ctx, self.ConfigObj, FORCE_REFRESH)
	assert.NoError(self.T(), err)

	// Check the master is removed.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		_, pres := self.master_dispatcher.GetHunt(self.Ctx, hunt_id)
		if pres {
			err = self.master_dispatcher.Refresh(self.Ctx, self.ConfigObj, FORCE_REFRESH)
			assert.NoError(self.T(), err)
		}

		return pres == false
	})

	// Check the minion is removed.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		_, pres := self.minion_dispatcher.GetHunt(self.Ctx, hunt_id)
		if pres {
			err = self.minion_dispatcher.Refresh(self.Ctx, self.ConfigObj, FORCE_REFRESH)
			assert.NoError(self.T(), err)
		}
		return pres == false
	})

	// Verify the files are gone from the filestore.
	test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()
	test_utils.GetMemoryDataStore(self.T(), self.ConfigObj).Debug(self.ConfigObj)
}

func TestHuntDispatcherTestSuite(t *testing.T) {
	suite.Run(t, &HuntDispatcherTestSuite{
		hunt_id: "H.1234",
	})
}
