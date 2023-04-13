package flows

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/result_sets/simple"
)

var (
	testArtifact = `
name: Test.Artifact
`
)

type TestSuite struct {
	test_utils.TestSuite
	client_id, flow_id string
}

func (self *TestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.HuntDispatcher = true

	self.TestSuite.SetupTest()
}

func (self *TestSuite) TestArtifactSource() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	_, err = repository.LoadYaml(testArtifact,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})

	assert.NoError(self.T(), err)

	file_store_factory := file_store.GetFileStore(self.ConfigObj)

	path_manager, err := artifacts.NewArtifactPathManager(self.Ctx,
		self.ConfigObj, self.client_id, self.flow_id,
		"Test.Artifact")
	assert.NoError(self.T(), err)

	// Append logs to messages from previous packets.
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, path_manager.Path(),
		nil, utils.SyncCompleter, true /* truncate */)
	assert.NoError(self.T(), err)

	for i := 0; i < 100; i++ {
		rs_writer.Write(ordereddict.NewDict().Set("Foo", i))
	}
	rs_writer.Close()

	ctx := context.Background()
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict().
			Set("ClientId", self.client_id).
			Set("FlowId", self.flow_id),
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	row_chan, err := breakIntoScopes(
		ctx, self.ConfigObj, scope,
		&ParallelPluginArgs{
			Artifact:  "Test.Artifact",
			FlowId:    self.flow_id,
			ClientId:  self.client_id,
			BatchSize: 10,
		})
	assert.NoError(self.T(), err)

	for args := range row_chan {
		start_row, _ := args.Get("StartRow")
		limit, _ := args.Get("Limit")
		fmt.Printf("Section %v-%v\n", start_row, limit)
	}

	vql, err := vfilter.Parse(`
SELECT * FROM parallelize(
     artifact='Test.Artifact',
     client_id=ClientId, flow_id=FlowId,
     batch=10, workers=10,
     query={
        SELECT Foo FROM source()
     })
`)
	assert.NoError(self.T(), err)

	result := make([]*ordereddict.Dict, 0)
	for row := range vql.Eval(ctx, scope) {
		result = append(result, row.(*ordereddict.Dict))
	}

	assert.Equal(self.T(), 100, len(result))

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()
}

func (self *TestSuite) TestHuntsSource() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	_, err = repository.LoadYaml(testArtifact, services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: true})

	assert.NoError(self.T(), err)
	ctx := context.Background()

	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(self.T(), err)

	hunt_id, err := hunt_dispatcher.CreateHunt(ctx,
		self.ConfigObj, acl_managers.NullACLManager{},
		&api_proto.Hunt{
			StartRequest: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"Test.Artifact"},
			},
		})
	assert.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	file_store_factory := file_store.GetFileStore(self.ConfigObj)
	hunt_path_manager := paths.NewHuntPathManager(hunt_id).Clients()
	hunt_rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, hunt_path_manager, nil,
		utils.SyncCompleter, true /* truncate */)

	for client_number := 0; client_number < 10; client_number++ {
		launcher.SetFlowIdForTests(fmt.Sprintf(
			"%s_%v", self.flow_id, client_number))

		client_id := fmt.Sprintf("%s_%v", self.client_id, client_number)
		flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx,
			self.ConfigObj, acl_managers.NullACLManager{},
			repository, &flows_proto.ArtifactCollectorArgs{
				ClientId:  client_id,
				Artifacts: []string{"Test.Artifact"},
			}, nil)
		assert.NoError(self.T(), err)

		hunt_rs_writer.Write(ordereddict.NewDict().
			Set("ClientId", client_id).
			Set("HuntId", hunt_id).
			Set("FlowId", flow_id).
			Set("_ts", 0).
			Set("Timestamp", 0))

		path_manager, err := artifacts.NewArtifactPathManager(self.Ctx,
			self.ConfigObj, client_id, flow_id, "Test.Artifact")
		assert.NoError(self.T(), err)

		// Append logs to messages from previous packets.
		rs_writer, err := result_sets.NewResultSetWriter(
			file_store_factory, path_manager.Path(),
			nil, utils.SyncCompleter, true /* truncate */)
		assert.NoError(self.T(), err)

		for i := 0; i < 100; i++ {
			rs_writer.Write(ordereddict.NewDict().
				Set("Foo", fmt.Sprintf("%v-%v", flow_id, i)))
		}
		rs_writer.Close()
	}

	hunt_rs_writer.Close()
	hunt_dispatcher.Refresh(self.ConfigObj)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict().Set("MyHuntId", hunt_id),
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	row_chan, err := breakIntoScopes(
		ctx, self.ConfigObj, scope,
		&ParallelPluginArgs{
			Artifact:  "Test.Artifact",
			HuntId:    hunt_id,
			BatchSize: 10,
		})
	assert.NoError(self.T(), err)

	sections := []string{}
	for args := range row_chan {
		start_row, _ := args.Get("StartRow")
		limit, _ := args.Get("Limit")
		flow_id, _ := args.Get("FlowId")
		sections = append(sections,
			fmt.Sprintf("Section %v: %v-%v\n", flow_id, start_row, limit))
	}

	// Stable sort the section list so we can goldie it.
	sort.Strings(sections)
	goldie.Assert(self.T(), "TestHuntsSource", json.MustMarshalIndent(sections))

	vql, err := vfilter.Parse(`
SELECT * FROM parallelize(
     artifact='Test.Artifact',
     hunt_id=MyHuntId,
     batch=10, workers=10,
     query={
        SELECT Foo FROM source()
     })
`)
	assert.NoError(self.T(), err)

	result := make([]*ordereddict.Dict, 0)
	for row := range vql.Eval(ctx, scope) {
		result = append(result, row.(*ordereddict.Dict))
	}

	assert.Equal(self.T(), 1000, len(result))
}

func TestParallelPlugin(t *testing.T) {
	suite.Run(t, &TestSuite{
		client_id: "C.123",
		flow_id:   "F.123",
	})
}
