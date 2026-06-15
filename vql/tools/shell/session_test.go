package shell

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/http_comms/e2e"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	vfilter "www.velocidex.com/golang/vfilter"

	// For query() plugin
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/protocols"
	_ "www.velocidex.com/golang/velociraptor/vql/tools"
)

// This test is an end to end test to ensure the shell session
// resumption works in real life.
type ShellSessionTestSuite struct {
	e2e.E2ETestSuite
}

func (self *ShellSessionTestSuite) TestBashShell() {
	defer self.StartServerAndClient()()

	artifact_name := "Linux.Sys.BashShell"

	// Schedule a collection on the client
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.ClientId,
		FlowId:    "/S",
		Artifacts: []string{artifact_name},
		Specs: []*flows_proto.ArtifactSpec{{
			Artifact: artifact_name,
			Parameters: &flows_proto.ArtifactParameters{
				Env: []*actions_proto.VQLEnv{{
					Key:   "Command",
					Value: "echo hello",
				}, {
					Key:   "CommandId",
					Value: "1",
				}},
			},
			MaxBatchWait: 1,
			MaxBatchRows: 1,
		}},
		Creator: utils.GetSuperuserName(self.ConfigObj),
	}

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		request, nil)
	assert.NoError(self.T(), err)

	flow_id, _ = utils.SplitSessionIdToParentAndChild(flow_id)

	var flow *api_proto.FlowDetails

	// Wait until we have two rows from the initial command
	vtesting.WaitUntil(50*time.Second, self.T(), func() bool {
		flow, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.ClientId, flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.TotalCollectedRows >= 2
	})

	golden := ordereddict.NewDict().Set(
		"Initial Command", self.getRows(artifact_name, flow_id))

	// Now relaunch the flow with more commands
	request.FlowId = flow_id + "/1"
	request.Specs[0].Parameters.Env = []*actions_proto.VQLEnv{{
		Key:   "Command",
		Value: "echo goodbye",
	}, {
		Key:   "CommandId",
		Value: "2",
	}}

	_, err = launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		request, nil)
	assert.NoError(self.T(), err)

	// Wait for the new results to be added.
	vtesting.WaitUntil(50*time.Second, self.T(), func() bool {
		flow, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.ClientId, flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.TotalCollectedRows >= 4
	})
	golden.Set("Second command", self.getRows(artifact_name, flow_id))
	goldie.Assert(self.T(), "TestBashShell", json.MustMarshalIndent(golden))
}

func (self *ShellSessionTestSuite) TestBashShellUnstateful() {
	defer self.StartServerAndClient()()

	artifact_name := "Linux.Sys.BashShell"

	// Schedule a collection on the client
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.ClientId,
		FlowId:    "/S",
		Artifacts: []string{artifact_name},
		Specs: []*flows_proto.ArtifactSpec{{
			Artifact: artifact_name,
			Parameters: &flows_proto.ArtifactParameters{
				Env: []*actions_proto.VQLEnv{{
					Key:   "Command",
					Value: "echo hello",
				}, {
					Key:   "CommandId",
					Value: "2",
				}, {
					Key:   "Stateful",
					Value: "N",
				}},
			},
			MaxBatchWait: 1,
			MaxBatchRows: 1,
		}},
		Creator: utils.GetSuperuserName(self.ConfigObj),
	}

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		request, nil)
	assert.NoError(self.T(), err)

	flow_id, _ = utils.SplitSessionIdToParentAndChild(flow_id)

	var flow *api_proto.FlowDetails

	// Wait until we have two rows from the initial command
	vtesting.WaitUntil(30*time.Second, self.T(), func() bool {
		flow, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.ClientId, flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.TotalCollectedRows >= 2
	})

	golden := ordereddict.NewDict().Set(
		"Initial Command", self.getRows(artifact_name, flow_id))

	// Now relaunch the flow with more commands
	request.FlowId = flow_id + "/1"
	request.Specs[0].Parameters.Env = []*actions_proto.VQLEnv{{
		Key:   "Command",
		Value: "echo goodbye",
	}, {
		Key:   "CommandId",
		Value: "3",
	}, {
		Key:   "Stateful",
		Value: "N",
	}}

	_, err = launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		request, nil)
	assert.NoError(self.T(), err)

	// Wait for the new results to be added.
	/*
		vtesting.WaitUntil(50*time.Second, self.T(), func() bool {
			flow, err = launcher.GetFlowDetails(
				self.Ctx, self.ConfigObj, services.GetFlowOptions{},
				self.ClientId, flow_id)
			assert.NoError(self.T(), err)

			return flow.Context.TotalCollectedRows >= 4
		})
	*/
	time.Sleep(time.Second * 4)
	golden.Set("Second command", self.getRows(artifact_name, flow_id))
	goldie.Assert(self.T(), "TestBashShellUnstateful",
		json.MustMarshalIndent(golden))
}

// Test that if the original session is expired, the new session
// resumes it correctly.
func (self *ShellSessionTestSuite) TestExpiredBashShell() {
	defer self.StartServerAndClient()()

	artifact_name := "Linux.Sys.BashShell"

	// Schedule a collection on the client
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.ClientId,
		FlowId:    "/S",
		Artifacts: []string{artifact_name},
		Specs: []*flows_proto.ArtifactSpec{{
			Artifact: artifact_name,
			Parameters: &flows_proto.ArtifactParameters{
				Env: []*actions_proto.VQLEnv{{
					Key:   "Command",
					Value: "echo hello",
				}, {
					Key:   "CommandId",
					Value: "3",
				}},
			},
			MaxBatchWait: 1,
			MaxBatchRows: 1,
		}},
		Creator: utils.GetSuperuserName(self.ConfigObj),
	}

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		request, nil)
	assert.NoError(self.T(), err)

	// The flow returned is the server side flow id.
	assert.True(self.T(), !strings.Contains(flow_id, "/"))

	var flow *api_proto.FlowDetails

	// Wait until we have two rows from the initial command
	vtesting.WaitUntil(30*time.Second, self.T(), func() bool {
		flow, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.ClientId, flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.TotalCollectedRows >= 2
	})

	golden := ordereddict.NewDict().Set(
		"Initial Command", self.getRows(artifact_name, flow_id))

	// Cancel the main flow
	_, err = launcher.CancelFlow(
		self.Ctx, self.ConfigObj, self.ClientId,
		flow_id, utils.GetSuperuserName(self.ConfigObj))
	assert.NoError(self.T(), err)

	// Wait until the client acks it.
	vtesting.WaitUntil(30*time.Second, self.T(), func() bool {
		flow, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.ClientId, flow_id)
		assert.NoError(self.T(), err)

		// Wait for the flow to be cancelled on the client side.
		return flow.Context.State == flows_proto.ArtifactCollectorContext_ERROR &&
			flow.Context.QueryStats[1].Status == crypto_proto.VeloStatus_GENERIC_ERROR
	})

	// Now relaunch the flow with more commands
	request.FlowId = fmt.Sprintf("%s/%d", flow_id, utils.GetGUID())
	request.Specs[0].Parameters.Env = []*actions_proto.VQLEnv{{
		Key:   "Command",
		Value: "echo goodbye",
	}, {
		Key:   "CommandId",
		Value: "4",
	}}

	_, err = launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		request, nil)
	assert.NoError(self.T(), err)

	// Wait for the new results to be added.
	vtesting.WaitUntil(50*time.Second, self.T(), func() bool {
		flow, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.ClientId, flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.State == flows_proto.ArtifactCollectorContext_IN_PROGRESS &&
			flow.Context.TotalCollectedRows >= 1
	})
	//time.Sleep(time.Hour)
	golden.Set("Second command", self.getRows(artifact_name, flow_id))
	goldie.Assert(self.T(), "TestExpiredBashShell", json.MustMarshalIndent(golden))
}

func (self *ShellSessionTestSuite) getRows(
	artifact_name, flow_id string) (res []vfilter.Row) {
	// Read the logs
	path_manager, err := artifacts.NewArtifactPathManager(
		self.Ctx, self.ConfigObj, self.ClientId, flow_id, artifact_name)
	assert.NoError(self.T(), err)

	file_store_factory := file_store.GetFileStore(self.ConfigObj)
	rs_reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())
	assert.NoError(self.T(), err)

	for row := range rs_reader.Rows(self.Ctx) {
		comm, _ := row.GetString("Command")
		comm_id, _ := row.GetString("CommandId")
		stdout, _ := row.GetString("Stdout")
		res = append(res, ordereddict.NewDict().
			Set("Command", comm).
			Set("CommandId", comm_id).
			Set("Stdout", stdout))
	}

	return res
}

func TestShellSession(t *testing.T) {
	// Check that CGO is enabled - this is required for sqlite
	// support.
	if runtime.GOOS != "linux" {
		t.Skip("Skipping shell session tests for linux.")
		return
	}

	res := &ShellSessionTestSuite{}
	data, err := utils.GzipUncompress(
		assets.FileArtifactsDefinitionsLinuxSysBashShellYaml)
	assert.NoError(t, err)

	res.Artifacts = []string{string(data)}

	suite.Run(t, res)
}
