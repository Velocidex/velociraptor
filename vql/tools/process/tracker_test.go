package process

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"

	_ "www.velocidex.com/golang/velociraptor/result_sets/simple"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/protocols"
)

type testCases_t struct {
	Name       string
	Mock       string
	UpdateMock string
	Query      string
	Clock      utils.Clock
}

var (
	stockUpdateTest = `
LET Tracker <= process_tracker(
cache=CacheFile,
sync_query={
  SELECT Pid AS id,
         Ppid AS parent_id,
         CreateTime AS start_time,
         dict(Name=Name) AS data
  FROM mock_pslist()
}, update_query={
  SELECT * FROM mock_update()
}, sync_period=500000)

// Wait for the tracker to be updated
LET _ <= mock_update_wait()

-- Pid 5 should be exited.
SELECT process_tracker_callchain(id=2)
FROM scope()
`

	stockSyncTest = `
LET Tracker <= process_tracker(
cache=CacheFile,
sync_query={
  SELECT Pid AS id,
         Ppid AS parent_id,
         CreateTime AS start_time,
         dict(Name=Name) AS data
  FROM mock_pslist()
}, sync_period=5)

LET _ <= mock_pslist_next()

-- First call Pid 5 is still around.
SELECT process_tracker_callchain(id=2).Data.Name
FROM scope()

LET _ <= mock_pslist_next()

-- Second call Pid 5 is exited - should mark Pid 5 as exited.
SELECT process_tracker_callchain(id=2)
FROM scope()
`

	overflowTest = `
LET Tracker <= process_tracker(
cache=CacheFile,
sync_query={
  SELECT Pid AS id,
         Ppid AS parent_id,
         CreateTime AS start_time,
         dict(Name=Name) AS data
  FROM mock_pslist()
}, update_query={
  SELECT * FROM mock_update()
}, sync_period=500000, max_size=10)

// Wait for the tracker to be updated
LET _ <= mock_update_wait()

SELECT * FROM  process_tracker_pslist()
ORDER BY Name
`

	testCases = []*testCases_t{
		{
			Name: "Parent Process Exiting (update)",
			Mock: `
[[{"Pid":2,"Name":"Process2","Ppid":5,"CreateTime": "2021-01-01T12:30Z"},
  {"Pid":5,"Name":"Process5","Ppid":1,"CreateTime": "2021-01-01T10:30Z"} ]]`,
			UpdateMock: `
[
 {"update_type":"exit","id":"5","end_time":"2021-01-01T12:30Z"}
]`,
			Query: stockUpdateTest,
			Clock: &utils.IncClock{NowTime: 1651000000},
		},
		{
			Name: "New Process (update)",
			Mock: `
[[{"Pid":1,"Name":"Process1","Ppid":0,"CreateTime": "2021-01-01T12:30Z"} ]]`,
			UpdateMock: `
[
  {"update_type":"start","id":2,"parent_id":1,"start_time":"2021-01-01T12:30Z",
   "data": {"Name": "Process2"}}
]`,
			Query: stockUpdateTest,
			Clock: &utils.IncClock{NowTime: 1651000000},
		},

		{
			Name: "New Process Start/End (update)",
			Mock: `
[[{"Pid":1,"Name":"Process1","Ppid":0,"CreateTime": "2021-01-01T12:30Z"} ]]`,
			UpdateMock: `
[
 {"update_type":"start","id":2,"parent_id":1,"start_time":"2021-01-01T12:30Z",
   "data": {"Name": "Process2"}},
 {"update_type":"exit","id":2,"end_time":"2021-01-01T14:30Z"}
]`,
			Query: stockUpdateTest,
			Clock: &utils.IncClock{NowTime: 1651000000},
		},

		{
			Name: "Parent Process Exiting (pslist)",
			// First pass has pid 2 and 5, second pass has only 2 (5
			// exited). Check that exit time is set properly.
			Mock: `
[
 [{"Pid":2,"Name":"Process2","Ppid":5,"CreateTime": "2021-01-01T12:30Z"},
  {"Pid":5,"Name":"Process5","Ppid":1,"CreateTime": "2021-01-01T10:30Z"}
 ],
 [{"Pid":2,"Name":"Process2","Ppid":5,"CreateTime": "2021-01-01T12:30Z"}]
]`,
			Query: stockSyncTest,
			Clock: &utils.IncClock{NowTime: 1651000000},
		},

		{
			Name: "Pid reuse",
			// Pid 2 is child of pid 5. First pass pid 5 has one
			// create time, second pass pid 5 has a different create
			// time (ie was reused). This should replace old pid 5
			// with guid and reparent pid 2 to the original pid 5.
			Mock: `
[
 [{"Pid":2,"Name":"Process2","Ppid":5,"CreateTime": "2021-01-01T12:30Z"},
  {"Pid":5,"Name":"Process5","Ppid":1,"CreateTime": "2021-01-01T10:30Z"}
 ],
 [{"Pid":2,"Name":"Process2","Ppid":5,"CreateTime": "2021-01-01T12:30Z"},
  {"Pid":5,"Name":"NewProcess5","Ppid":1,"CreateTime": "2021-01-11T10:30Z"}
 ]
]`,
			Query: stockSyncTest,
			Clock: &utils.IncClock{NowTime: 1651000000},
		},

		{
			Name: "Pid reuse with missing parent",
			// First sync has a process with the missing parent (pid 5
			// is not known). On the second sync that process is
			// reused with a new process with pid 5 (and a different
			// parent 10). The tracker should **not** associate the
			// 2->5->10 chain sincce this is not correct.
			Mock: `
[
 [{"Pid":2,"Name":"Process2","Ppid":5,"CreateTime": "2021-01-01T12:30Z"}
 ],
 [{"Pid":5,"Name":"Process2","Ppid":10,"CreateTime": "2021-01-01T12:30Z"},
  {"Pid":2,"Name":"NewProcess5","Ppid":5,"CreateTime": "2021-01-01T12:30Z"}
 ]
]`,
			Query: stockSyncTest,
			Clock: &utils.IncClock{NowTime: 1651000000},
		},

		{
			Name: "Spoof (update)",
			Mock: `
[
 [{"Pid":2,"Name":"Process2","Ppid":5,"CreateTime": "2021-01-01T12:30Z"},
  {"Pid":5,"Name":"Process5","Ppid":1,"CreateTime": "2021-01-01T10:30Z"}
 ]
]`,
			UpdateMock: `
[
 {"update_type":"start","id":5,"parent_id":1,"start_time":"2021-01-11T12:30Z",
   "data": {"Name": "NewProcess5"}}
]`,
			Query: stockUpdateTest,
			Clock: &utils.IncClock{NowTime: 1651000000},
		},
		{
			Name: "Overflow cache",
			Mock: `
[
 [
 {"Pid":1,"Name":"Process1","Ppid":5,"CreateTime": "2021-01-01T12:30:01Z"},
 {"Pid":2,"Name":"Process2","Ppid":5,"CreateTime": "2021-01-01T12:30:02Z"},
 {"Pid":3,"Name":"Process3","Ppid":5,"CreateTime": "2021-01-01T12:30:03Z"}
 ]
]`,
			UpdateMock: `
[
 {"update_type":"start","id":14,"parent_id":1,"start_time":"2021-01-11T12:31Z",
   "data": {"Name": "NewProcess14"}},
 {"update_type":"start","id":15,"parent_id":1,"start_time":"2021-01-11T12:32Z",
   "data": {"Name": "NewProcess15"}},
 {"update_type":"start","id":16,"parent_id":1,"start_time":"2021-01-11T12:33Z",
   "data": {"Name": "NewProcess16"}},
 {"update_type":"start","id":17,"parent_id":1,"start_time":"2021-01-11T12:34Z",
   "data": {"Name": "NewProcess17"}},
 {"update_type":"start","id":18,"parent_id":1,"start_time":"2021-01-11T12:31Z",
   "data": {"Name": "NewProcess18"}},
 {"update_type":"start","id":19,"parent_id":1,"start_time":"2021-01-11T12:32Z",
   "data": {"Name": "NewProcess19"}},
 {"update_type":"start","id":20,"parent_id":1,"start_time":"2021-01-11T12:33Z",
   "data": {"Name": "NewProcess20"}},
 {"update_type":"start","id":21,"parent_id":1,"start_time":"2021-01-11T12:34Z",
   "data": {"Name": "NewProcess21"}}

]`,
			Query: overflowTest,
			Clock: &utils.IncClock{NowTime: 1651000000},
		},
	}
)

type ProcessTrackerTestSuite struct {
	test_utils.TestSuite

	name           string
	cache_filename string
}

func (self *ProcessTrackerTestSuite) runTC(
	ctx context.Context, test_case *testCases_t) []*ordereddict.Dict {
	closer := utils.MockTime(test_case.Clock)
	defer closer()

	loadMockPlugin(self.T(), test_case.Mock)
	if test_case.UpdateMock != "" {
		loadMockUpdatePlugin(self.T(), test_case.UpdateMock)
	}

	// Just build a standard scope.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict().
			Set("LRU_Debug", true).
			Set("CacheFile", self.cache_filename),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	rows := make([]*ordereddict.Dict, 0)
	mvql, err := vfilter.MultiParse(test_case.Query)
	for _, vql := range mvql {
		for row := range vql.Eval(ctx, scope) {
			rows = append(rows, vfilter.RowToDict(ctx, scope, row))
		}
	}
	scope.Close()

	return rows

}

func (self *ProcessTrackerTestSuite) TestProcessTracker() {
	assert.Retry(self.T(), 3, time.Second, self._TestProcessTracker)
}

func (self *ProcessTrackerTestSuite) _TestProcessTracker(r *assert.R) {

	results := ordereddict.NewDict()

	ctx, cancel := context.WithTimeout(self.Ctx, 10000000*time.Second)
	defer cancel()

	for idx, test_case := range testCases {
		if false && idx != 1 {
			continue
		}

		results.Set(test_case.Name, self.runTC(ctx, test_case))
	}

	normalize := regexp.MustCompile("2022-04-26T.*?Z").ReplaceAllString(
		string(json.MustMarshalIndent(results)), "2022-04-26TZ")

	goldie.Retry(r, self.T(),
		self.name+"TestProcessTracker", []byte(normalize))
}

func (self *ProcessTrackerTestSuite) TestForkBomb() {
	vql_subsystem.OverridePlugin(&_MockForkBombUpdate{})

	query := `
LET Tracker <= process_tracker(
cache=CacheFile,
update_query={
  SELECT * FROM mock_forkbomb(depth=Depth)
}, sync_period=500000, max_size=100000)

// Wait for the tracker to be updated
LET _ <= mock_update_wait()

-- Pid 5 should be exited.
LET Tree <= process_tracker_tree(id=1)
LET Serialized <= serialize(item=Tree)
LET JSONTreeSize <= len(list=Serialized)
LET ProcessCount = SELECT count() AS Count
  FROM process_tracker_pslist() GROUP BY 1

SELECT len(list=split(string=Serialized, sep_string='"name"')) - 1 AS Number,
       JSONTreeSize, ProcessCount.Count[0] AS TrackedCount
FROM scope()
`
	golden := ""

	for depth := 1; depth < 8; depth++ {
		// Just build a standard scope.
		builder := services.ScopeBuilder{
			Config:     self.ConfigObj,
			ACLManager: acl_managers.NullACLManager{},
			Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
			Env: ordereddict.NewDict().
				Set("LRU_Debug", true).
				Set("CacheFile", self.cache_filename).
				Set("Depth", depth),
		}

		manager, err := services.GetRepositoryManager(self.ConfigObj)
		assert.NoError(self.T(), err)

		scope := manager.BuildScope(builder)

		mvql, err := vfilter.MultiParse(query)
		for _, vql := range mvql {
			for row := range vql.Eval(self.Ctx, scope) {
				golden += json.StringIndent(row)
			}
		}
		scope.Close()
	}

	goldie.Assert(self.T(), self.name+"TestForkBomb", []byte(golden))
}

func TestProcessTrackerMemory(t *testing.T) {
	suite.Run(t, &ProcessTrackerTestSuite{
		name: "ProcessTrackerMemoryTest_",
	})
}

type ProcessTrackerTestSuiteFile struct {
	ProcessTrackerTestSuite
}

func (self *ProcessTrackerTestSuiteFile) SetupTest() {
	self.ProcessTrackerTestSuite.SetupTest()

	// For testing we use an in memory sqlite database so it is a bit
	// faster. In practice this will be a real file.
	self.cache_filename = ":memory:"
}

func TestProcessTrackerFile(t *testing.T) {
	// Check that CGO is enabled - this is required for sqlite
	// support.
	if !utils.CGO_ENABLED {
		t.Skip("Skipping disk based process tracker because CGO is disabled.")
		return
	}

	suite.Run(t, &ProcessTrackerTestSuiteFile{
		ProcessTrackerTestSuite{
			name: "ProcessTrackerFileTest_",
		},
	})
}

type _MockForkBombUpdateArgs struct {
	Depth int64 `vfilter:"required,field=depth"`
}

type _MockForkBombUpdate struct{}

func (self _MockForkBombUpdate) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "mock_forkbomb",
	}
}

func (self _MockForkBombUpdate) Call(
	ctx context.Context, scope types.Scope,
	args *ordereddict.Dict) <-chan types.Row {

	output_chan := make(chan types.Row)

	mu.Lock()
	plugin_update_done = false
	mu.Unlock()

	go func() {
		defer close(output_chan)

		arg := &_MockForkBombUpdateArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("mock_forkbomb: %s", err.Error())
			return
		}

		counter := &utils.Counter{}
		fork(int(arg.Depth), counter.Get(), 3, output_chan, counter)

		scope.Log("Fork simulation ended with %v processes", counter.Get())

		// Signal to the query it may proceed.
		mu.Lock()
		plugin_update_done = true
		mu.Unlock()

		// Wait here until the end of the test.
		<-ctx.Done()
	}()

	return output_chan
}

// Emulate the pid creating count children. Each of these children
// will also create count children up to the depth specified.
func fork(depth, pid, count int, output_chan chan types.Row,
	counter *utils.Counter) {

	if depth < 0 {
		return
	}

	for i := 0; i < count; i++ {
		counter.Inc()
		record := ordereddict.NewDict().
			Set("update_type", "start").
			Set("id", counter.Get()).
			Set("parent_id", pid).
			Set("start_time", time.Unix(100000000+int64(counter.Get()), 0)).
			Set("data", ordereddict.NewDict().
				Set("Pid", counter.Get()).
				Set("Ppid", pid).
				Set("Name", fmt.Sprintf("Pid%v", counter.Get())))
		output_chan <- record

		fork(depth-1, counter.Get(), count, output_chan, counter)
	}
}
