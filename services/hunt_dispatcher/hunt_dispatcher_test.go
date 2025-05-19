package hunt_dispatcher_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
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
	self.ConfigObj.Defaults.HuntDispatcherRefreshSec = 1

	self.LoadArtifactsIntoConfig([]string{`
name: Server.Internal.HuntUpdate
type: INTERNAL
`})

	self.time_closer = utils.MockTime(&utils.IncClock{})

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	for i := 0; i < 5; i++ {
		now := utils.GetTime().Now().Unix()
		hunt_obj := &api_proto.Hunt{
			HuntId:    fmt.Sprintf("H.%d", i),
			State:     api_proto.Hunt_RUNNING,
			Version:   now,
			StartTime: uint64(now),
			Expires:   uint64(now+10) * 1000000,
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
	self.master_dispatcher.Store.(*hunt_dispatcher.HuntStorageManagerImpl).I_am_master = true

	minion_dispatcher, err := hunt_dispatcher.NewHuntDispatcher(
		self.Ctx, self.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)
	self.minion_dispatcher = minion_dispatcher.(*hunt_dispatcher.HuntDispatcher)
	self.minion_dispatcher.Store.(*hunt_dispatcher.HuntStorageManagerImpl).I_am_master = false

	// Wait until the hunt dispatchers are fully loaded.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		hunt, pres := self.master_dispatcher.GetHunt(self.Ctx, "H.4")
		return pres && hunt.HuntId == "H.4"
	})

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		hunt, pres := self.minion_dispatcher.GetHunt(self.Ctx, "H.4")
		return pres && hunt.HuntId == "H.4"
	})
}

func (self *HuntDispatcherTestSuite) TestLoadingFromDisk() {
	// All hunts are now running.
	hunts := self.getAllHunts()

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
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
	hunt_obj, pres = self.minion_dispatcher.GetHunt(self.Ctx, "H.1")
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

	storage := hunt_dispatcher.NewHuntStorageManagerImpl(self.ConfigObj)

	// But not immediately visible in minion
	err := storage.(*hunt_dispatcher.HuntStorageManagerImpl).
		LoadHuntsFromIndex(self.Ctx, self.ConfigObj)
	assert.NoError(self.T(), err)
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
	hunt_path_manager := paths.NewHuntPathManager("H.2")
	hunt_obj := &api_proto.Hunt{}
	err = db.GetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_RUNNING)

	// Now modify a hunt with services.HuntPropagateChanges
	ctx := self.Ctx
	modification := self.master_dispatcher.ModifyHuntObject(ctx, "H.2",
		func(hunt *api_proto.Hunt) services.HuntModificationAction {
			hunt.State = api_proto.Hunt_STOPPED
			return services.HuntPropagateChanges
		})

	// Changes may not visible in the data store immediately.
	assert.Equal(self.T(), modification, services.HuntPropagateChanges)

	// But they should be visible in master
	hunt_obj, pres := self.master_dispatcher.GetHunt(self.Ctx, "H.2")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_STOPPED)

	// And eventually also visible in minion
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

	now := utils.GetTime().Now().Unix()
	acl_manager := acl_managers.NullACLManager{}

	hunt_obj, err := self.master_dispatcher.CreateHunt(self.Ctx, self.ConfigObj,
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

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		hunt_obj, pres := self.master_dispatcher.GetHunt(self.Ctx, hunt_id)
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

	// Check the master is removed.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		_, pres := self.master_dispatcher.GetHunt(self.Ctx, hunt_id)
		return pres == false
	})

	// Check the minion is removed.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		_, pres := self.minion_dispatcher.GetHunt(self.Ctx, hunt_id)
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
