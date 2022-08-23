package process

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"

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
LET Tracker <= process_tracker(sync_query={
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
LET Tracker <= process_tracker(sync_query={
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

	testCases = []testCases_t{
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
			Clock: &utils.RealClock{},
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
			Clock: &utils.RealClock{},
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
			Clock: &utils.RealClock{},
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
	}
)

type ProcessTrackerTestSuite struct {
	test_utils.TestSuite
}

func (self *ProcessTrackerTestSuite) TestProcessTracker() {
	results := ordereddict.NewDict()

	ctx, cancel := context.WithTimeout(self.Ctx, 10*time.Second)
	defer cancel()

	for idx, test_case := range testCases {
		//if idx != 5 {
		//	continue
		//}

		_ = idx
		SetClock(test_case.Clock)

		loadMockPlugin(self.T(), test_case.Mock)
		if test_case.UpdateMock != "" {
			loadMockUpdatePlugin(self.T(), test_case.UpdateMock)
		}

		// Just build a standard scope.
		builder := services.ScopeBuilder{
			Config:     self.ConfigObj,
			ACLManager: acl_managers.NullACLManager{},
			Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		}

		manager, err := services.GetRepositoryManager(self.ConfigObj)
		assert.NoError(self.T(), err)

		scope := manager.BuildScope(builder)
		defer scope.Close()

		rows := make([]*ordereddict.Dict, 0)
		mvql, err := vfilter.MultiParse(test_case.Query)
		for _, vql := range mvql {
			for row := range vql.Eval(ctx, scope) {
				rows = append(rows, vfilter.RowToDict(ctx, scope, row))
			}
		}

		results.Set(test_case.Name, rows)
	}

	normalize := regexp.MustCompile("2022-04-26T.*?Z").ReplaceAllString(
		string(json.MustMarshalIndent(results)), "2022-04-26TZ")

	goldie.Assert(self.T(), "TestProcessTracker", []byte(normalize))
}

func TestProcessTracker(t *testing.T) {
	suite.Run(t, &ProcessTrackerTestSuite{})
}
