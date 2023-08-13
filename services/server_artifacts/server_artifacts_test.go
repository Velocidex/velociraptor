package server_artifacts_test

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/functions"
)

type ServerArtifactsTestSuite struct {
	test_utils.TestSuite
}

func (self *ServerArtifactsTestSuite) SetupSuite() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.ServerArtifacts = true
}

func (self *ServerArtifactsTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	// Create an administrator user
	err := services.GrantRoles(self.ConfigObj, "admin", []string{"administrator"})
	assert.NoError(self.T(), err)
}

func (self *ServerArtifactsTestSuite) ScheduleAndWait(
	name, user, flow_id string,
	wg *sync.WaitGroup, // If set we signal this wg when we finished scheduling.
) *api_proto.FlowDetails {
	ctx := self.Ctx

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)

	var mu sync.Mutex
	complete_flow_id := ""

	err := journal.WatchQueueWithCB(self.Sm.Ctx, self.ConfigObj, self.Sm.Wg,
		"System.Flow.Completion", "ServerArtifactsTestSuite", func(
			ctx context.Context,
			ConfigObj *config_proto.Config,
			row *ordereddict.Dict) error {
			flow, pres := row.Get("Flow")
			if pres {
				mu.Lock()
				complete_flow_id = flow.(*flows_proto.ArtifactCollectorContext).SessionId
				mu.Unlock()
			}
			return nil
		})
	assert.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	acl_manager := acl_managers.NewServerACLManager(self.ConfigObj, user)

	launcher.SetFlowIdForTests(flow_id)

	// Schedule a job for the server runner.
	flow_id, err = launcher.ScheduleArtifactCollection(
		self.Sm.Ctx, self.ConfigObj, acl_manager,
		repository, &flows_proto.ArtifactCollectorArgs{
			Creator:   user,
			ClientId:  "server",
			Artifacts: []string{name},
		}, func() {
			// Notify it about the new job
			notifier, err := services.GetNotifier(self.ConfigObj)
			assert.NoError(self.T(), err)

			err = notifier.NotifyListener(ctx, self.ConfigObj, "server", "")
			assert.NoError(self.T(), err)
		})
	assert.NoError(self.T(), err)

	if wg != nil {
		wg.Done()
	}

	// Wait for the collection to complete
	var details *api_proto.FlowDetails
	vtesting.WaitUntil(time.Second*50, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		details, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, "server", flow_id)
		assert.NoError(self.T(), err)

		return details.Context.State != flows_proto.ArtifactCollectorContext_RUNNING
	})

	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()
		return complete_flow_id == flow_id
	})

	return details
}

func (self *ServerArtifactsTestSuite) TestServerArtifacts() {
	self.LoadArtifacts(`
name: Test1
type: SERVER
sources:
- query: SELECT "Foo" FROM scope()
`)
	details := self.ScheduleAndWait("Test1", "admin", "F.1234", nil)

	// One row is collected
	assert.Equal(self.T(), uint64(1), details.Context.TotalCollectedRows)

	// How long we took to run - should be immediate
	run_time := (details.Context.ActiveTime - details.Context.StartTime) / 1000000
	assert.True(self.T(), run_time < 2)
}

func (self *ServerArtifactsTestSuite) TestServerArtifactsCancellation() {
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	self.LoadArtifacts(`
name: Test1
type: SERVER
sources:
- query: SELECT sleep(time=10000) FROM scope()
`)

	cancel_mu := &sync.Mutex{}
	var details *api_proto.FlowDetails

	schedule_wg := &sync.WaitGroup{}
	schedule_wg.Add(1)

	go func() {
		flow_details := self.ScheduleAndWait(
			"Test1", "admin", "F.1234", schedule_wg)

		cancel_mu.Lock()
		details = flow_details
		cancel_mu.Unlock()
	}()

	schedule_wg.Wait()

	// Wait for the flow to be created
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		_, err := launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, "server", "F.1234")
		return err == nil
	})

	// cancel the flow
	resp, err := launcher.CancelFlow(
		self.Ctx, self.ConfigObj, "server", "F.1234", "admin")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), resp.FlowId, "F.1234")

	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		cancel_mu.Lock()
		defer cancel_mu.Unlock()

		return details != nil
	})
	assert.Equal(self.T(), "Cancelled by admin", details.Context.Status)
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR,
		details.Context.State)
}

func (self *ServerArtifactsTestSuite) TestServerArtifactsRowLimit() {
	self.LoadArtifacts(`
name: Test1
type: SERVER
sources:
- query: SELECT _value FROM range(end=100)
resources:
  max_rows: 10
`)
	details := self.ScheduleAndWait("Test1", "admin", "F.1234", nil)

	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR,
		details.Context.State)
	assert.Equal(self.T(), "Row limit exceeded", details.Context.Status)
	assert.True(self.T(), details.Context.TotalCollectedRows < 20)
	assert.True(self.T(), details.Context.TotalCollectedRows >= 10)

	// How long we took to run - should be immediate
	run_time := (details.Context.ActiveTime - details.Context.StartTime) / 1000000
	assert.True(self.T(), run_time < 2)
}

func (self *ServerArtifactsTestSuite) TestServerArtifactWithUpload() {
	self.LoadArtifacts(`
name: TestUpload
type: SERVER
sources:
- query: |
     SELECT upload(accessor="data",
                   file="Hello world",
                   name="test.txt")
     FROM scope()
`)
	details := self.ScheduleAndWait("TestUpload", "admin", "F.1234", nil)

	// One row is collected
	assert.Equal(self.T(), uint64(1), details.Context.TotalCollectedRows)
	assert.Equal(self.T(), uint64(1), details.Context.TotalUploadedFiles)
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_FINISHED, details.Context.State)

	// How long we took to run - should be immediate
	run_time := (details.Context.ActiveTime - details.Context.StartTime) / 1000000
	assert.True(self.T(), run_time < 2)

	flow_path_manager := paths.NewFlowPathManager(
		"server", details.Context.SessionId)
	log_data := test_utils.FileReadAll(self.T(), self.ConfigObj,
		flow_path_manager.Log())
	assert.Contains(self.T(), log_data,
		"Uploaded /test.txt")

	// Make sure the upload data is stored in the upload file.
	uploads_data := test_utils.FileReadAll(self.T(), self.ConfigObj,
		flow_path_manager.UploadMetadata())
	assert.Contains(self.T(), uploads_data,
		`"_Components":["clients","server","collections","F.1234","uploads","test.txt"]`)

	// Now read the uploaded file.
	data := test_utils.FileReadAll(self.T(), self.ConfigObj,
		path_specs.NewUnsafeFilestorePath(
			"clients", "server", "collections", "F.1234", "uploads", "test.txt").
			SetType(api.PATH_TYPE_FILESTORE_ANY))
	assert.Equal(self.T(), "Hello world", string(data))
}

func (self *ServerArtifactsTestSuite) TestServerArtifactWithUploadDeduplication() {
	self.LoadArtifacts(`
name: TestUploadMany
type: SERVER
sources:
- query: |
     SELECT upload(accessor="data",
                   file="Hello world",
                   name="test_many.txt")
     FROM range(end=10)
`)
	details := self.ScheduleAndWait("TestUploadMany", "admin", "F.1234", nil)

	// 10 rows are collected
	assert.Equal(self.T(), uint64(10), details.Context.TotalCollectedRows)

	// But only one upload is actually made
	assert.Equal(self.T(), uint64(1), details.Context.TotalUploadedFiles)
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_FINISHED, details.Context.State)

	// Make sure the upload data is stored in the upload file.
	flow_path_manager := paths.NewFlowPathManager(
		"server", details.Context.SessionId)
	uploads_data := test_utils.FileReadAll(self.T(), self.ConfigObj,
		flow_path_manager.UploadMetadata())

	assert.Contains(self.T(), uploads_data,
		`"_Components":["clients","server","collections","F.1234","uploads","test_many.txt"]`)

	// There is only one uploaded file in the uploads file
	assert.Equal(self.T(), 1, len(strings.Split("\n", uploads_data)))

	// Now read the uploaded file.
	data := test_utils.FileReadAll(self.T(), self.ConfigObj,
		path_specs.NewUnsafeFilestorePath(
			"clients", "server", "collections", "F.1234",
			"uploads", "test_many.txt").
			SetType(api.PATH_TYPE_FILESTORE_ANY))
	assert.Equal(self.T(), "Hello world", string(data))
}

// An artifact with two sources - one will produce an error. The
// entire collection should fail but will have 2 rows returned.
func (self *ServerArtifactsTestSuite) TestServerArtifactsMultiSource() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer closer()

	self.LoadArtifacts(`
name: TestMultiSource
type: SERVER
sources:
- name: Source1
  precondition: SELECT 1 FROM scope()
  query: |
    SELECT "Foo", log(message="Oops", level="ERROR") AS Error
    FROM scope()

- name: Source2
  precondition: SELECT 1 FROM scope()
  query: SELECT "Foo" FROM scope()
`)
	details := self.ScheduleAndWait("TestMultiSource", "admin", "F.1234", nil)

	// Two rows are collected
	assert.Equal(self.T(), uint64(2), details.Context.TotalCollectedRows)

	// The collection is marked as an error because one of the queries
	// is an error.
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR,
		details.Context.State)

	// Two QueryStats are provided
	assert.Equal(self.T(), 2, len(details.Context.QueryStats))

	// Sort those for golden comparison
	sort.Slice(details.Context.QueryStats, func(i, j int) bool {
		return details.Context.QueryStats[i].ErrorMessage <
			details.Context.QueryStats[j].ErrorMessage
	})
	sort.Strings(details.Context.ArtifactsWithResults)

	goldie.Assert(self.T(), "TestMultiSource",
		json.MustMarshalIndent(details.Context))
}

// Multiple sources in the same precondition run serially
func (self *ServerArtifactsTestSuite) TestServerArtifactsMultiSourceSerial() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer closer()

	self.LoadArtifacts(`
name: TestMultiSourceSerial
type: SERVER
sources:
- name: Source1
  query: |
    SELECT "Foo", log(message="Oops", level="ERROR") AS Error
    FROM scope()

- name: Source2
  query: SELECT "Foo" FROM scope()
`)
	details := self.ScheduleAndWait("TestMultiSourceSerial", "admin", "F.1234", nil)

	// Two rows are collected
	assert.Equal(self.T(), uint64(2), details.Context.TotalCollectedRows)

	// The collection is marked as an error because one of the queries
	// is an error.
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR,
		details.Context.State)

	// One QueryStats because there is only one query
	assert.Equal(self.T(), 1, len(details.Context.QueryStats))

	goldie.Assert(self.T(), "TestMultiSourceSerial",
		json.MustMarshalIndent(details.Context))
}

func (self *ServerArtifactsTestSuite) TestServerArtifactsBytesLimit() {
	self.LoadArtifacts(`
name: Test1
type: SERVER
sources:
- query: |
    -- Need to store to different files or the upload will be deduplicated.
    SELECT upload(accessor="data",
                  file="Hello world",
                  name=format(format="test%d.txt", args=_value)) AS Upload
    FROM range(end=100)
resources:
  max_upload_bytes: 20
`)
	details := self.ScheduleAndWait("Test1", "admin", "F.1234", nil)

	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR,
		details.Context.State)
	assert.Equal(self.T(), "Byte limit exceeded", details.Context.Status)
	assert.True(self.T(), details.Context.TotalUploadedBytes < 30)
	assert.True(self.T(), details.Context.TotalUploadedBytes >= 10)

	// How long we took to run - should be immediate
	run_time := (details.Context.ActiveTime - details.Context.StartTime) / 1000000
	assert.True(self.T(), run_time < 2)
}

// Collect a long lived artifact with specified timeout.
func (self *ServerArtifactsTestSuite) TestServerArtifactsTimeout() {
	self.LoadArtifacts(`
name: Test2
type: SERVER
resources:
  timeout: 1
sources:
- query: SELECT sleep(time=200) FROM scope()
`)

	details := self.ScheduleAndWait("Test2", "admin", "F.1234", nil)

	// No rows are collected because the query timed out.
	assert.Equal(self.T(), uint64(0), details.Context.TotalCollectedRows)

	// How long we took to run - should be around 1 second
	run_time := (details.Context.ActiveTime - details.Context.StartTime) / 1000000
	assert.True(self.T(), run_time < 3)
	assert.True(self.T(), run_time >= 1)

	flow_path_manager := paths.NewFlowPathManager(
		"server", details.Context.SessionId)
	log_data := test_utils.FileReadAll(self.T(), self.ConfigObj,
		flow_path_manager.Log())
	assert.Contains(self.T(), log_data, "Query timed out after ")
}

// The server artifact runner impersonates the flow creator for ACL
// checks - this makes it safe for low privilege users to run some
// server artifacts that accommodate their access levels, but stops
// them from escalating to higher permissions.
func (self *ServerArtifactsTestSuite) TestServerArtifactsACLs() {

	// The info plugin requires MACHINE_STATE permission
	self.LoadArtifacts(`
name: Test
type: SERVER
sources:
- query: SELECT * FROM info()
`)

	details := self.ScheduleAndWait("Test", "admin", "F.1234", nil)

	// Admin user should be able to collect since it has EXECVE
	assert.Equal(self.T(), uint64(1), details.Context.TotalCollectedRows)

	// Create a reader user called gumby - reader role lacks the
	// MACHINE_STATE permission.
	err := services.GrantRoles(self.ConfigObj, "gumby", []string{"reader"})
	assert.NoError(self.T(), err)

	details = self.ScheduleAndWait("Test", "gumby", "F.1234", nil)

	// Gumby user has no permissions to run the artifact.
	assert.Equal(self.T(), uint64(0), details.Context.TotalCollectedRows)

	flow_path_manager := paths.NewFlowPathManager(
		"server", details.Context.SessionId)
	log_data := test_utils.FileReadAll(self.T(), self.ConfigObj,
		flow_path_manager.Log())
	assert.Contains(self.T(), log_data, "Permission denied: [MACHINE_STATE]")
}

func TestServerArtifacts(t *testing.T) {
	suite.Run(t, &ServerArtifactsTestSuite{})
}
