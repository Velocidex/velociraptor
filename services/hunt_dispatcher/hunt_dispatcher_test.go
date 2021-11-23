package hunt_dispatcher

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/frontend"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type HuntDispatcherTestSuite struct {
	test_utils.TestSuite

	hunt_id string

	master_dispatcher *HuntDispatcher
	minion_dispatcher *HuntDispatcher
}

func (self *HuntDispatcherTestSuite) SetupTest() {
	self.TestSuite.SetupTest()
	self.LoadArtifacts([]string{`
name: Server.Internal.HuntUpdate
type: INTERNAL
`})

	Clock = &utils.IncClock{}

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	for i := 0; i < 5; i++ {
		now := Clock.Now().Unix()
		hunt_obj := &api_proto.Hunt{
			HuntId:    fmt.Sprintf("H.%d", i),
			State:     api_proto.Hunt_RUNNING,
			Version:   now,
			StartTime: uint64(now),
		}
		hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
		assert.NoError(self.T(),
			db.SetSubject(self.ConfigObj,
				hunt_path_manager.Path(), hunt_obj))
	}

	require.NoError(self.T(), self.Sm.Start(frontend.StartFrontendService))

	// Make a master and minion dispatchers.
	require.NoError(self.T(), self.Sm.Start(StartHuntDispatcher))
	self.master_dispatcher = services.GetHuntDispatcher().(*HuntDispatcher)
	self.master_dispatcher.i_am_master = true

	require.NoError(self.T(), self.Sm.Start(StartHuntDispatcher))
	self.minion_dispatcher = services.GetHuntDispatcher().(*HuntDispatcher)
	self.minion_dispatcher.i_am_master = false

	// Wait until the hunt dispatchers are fully loaded.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		hunt, pres := self.master_dispatcher.GetHunt("H.4")
		return pres && hunt.HuntId == "H.4"
	})

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		hunt, pres := self.minion_dispatcher.GetHunt("H.4")
		return pres && hunt.HuntId == "H.4"
	})
}

func (self *HuntDispatcherTestSuite) TestLoadingFromDisk() {
	// All hunts are now running.
	hunts := self.getAllHunts()
	assert.Equal(self.T(), len(hunts), 5)
	for _, h := range hunts {
		assert.Equal(self.T(), h.State, api_proto.Hunt_RUNNING)
	}
}

func (self *HuntDispatcherTestSuite) TestModifyingHuntFlushToDatastore() {
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Modify a hunt and flush to datastore immediately
	modification := self.master_dispatcher.ModifyHunt("H.1",
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
	hunt_obj, pres := self.master_dispatcher.GetHunt("H.1")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_STOPPED)

	// But not immediately visible in minion
	hunt_obj, pres = self.minion_dispatcher.GetHunt("H.1")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_RUNNING)
}

func (self *HuntDispatcherTestSuite) TestModifyingHuntPropagateChanges() {
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Now modify a hunt with services.HuntPropagateChanges
	modification := self.master_dispatcher.ModifyHunt("H.2",
		func(hunt *api_proto.Hunt) services.HuntModificationAction {
			hunt.State = api_proto.Hunt_STOPPED
			return services.HuntPropagateChanges
		})

	// Changes are not visible in the data store immediately.
	assert.Equal(self.T(), modification, services.HuntPropagateChanges)
	hunt_path_manager := paths.NewHuntPathManager("H.2")
	hunt_obj := &api_proto.Hunt{}
	err = db.GetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_RUNNING)

	// But they should be visible in master
	hunt_obj, pres := self.master_dispatcher.GetHunt("H.2")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_STOPPED)

	// And eventually also visible in minion
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		hunt_obj, pres = self.minion_dispatcher.GetHunt("H.2")
		return pres && hunt_obj.State == api_proto.Hunt_STOPPED
	})
}

func (self *HuntDispatcherTestSuite) getAllHunts() []*api_proto.Hunt {
	// Get the list of all hunts
	hunts := []*api_proto.Hunt{}
	err := self.master_dispatcher.ApplyFuncOnHunts(
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

func TestHuntDispatcherTestSuite(t *testing.T) {
	suite.Run(t, &HuntDispatcherTestSuite{
		hunt_id: "H.1234",
	})
}
