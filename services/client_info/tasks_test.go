package client_info_test

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *ClientInfoTestSuite) TestQueueMessages() {
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	message1 := &crypto_proto.VeloMessage{Source: "Server", SessionId: "1"}
	err = client_info_manager.QueueMessageForClient(
		context.Background(),
		self.client_id, message1,
		services.NOTIFY_CLIENT, utils.BackgroundWriter)
	assert.NoError(self.T(), err)

	manager := client_info_manager.(*client_info.ClientInfoManager)

	// Wait here until the tasks is visible.
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		tasks, err := manager.PeekClientTasks(context.Background(), self.client_id)
		assert.NoError(self.T(), err)
		return len(tasks) == 1 && proto.Equal(tasks[0], message1)
	})

	// We did not lease, so the tasks are not removed, but this
	// time we will lease.
	tasks, err := manager.GetClientTasks(context.Background(), self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 1)

	// No tasks available.
	tasks, err = manager.PeekClientTasks(context.Background(), self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 0)
}

func (self *ClientInfoTestSuite) TestFastQueueMessages() {
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	written := []*crypto_proto.VeloMessage{}

	for i := 0; i < 10; i++ {
		message := &crypto_proto.VeloMessage{Source: "Server", SessionId: fmt.Sprintf("%d", i)}
		err := client_info_manager.QueueMessageForClient(
			context.Background(),
			self.client_id, message,
			services.NOTIFY_CLIENT, utils.BackgroundWriter)
		assert.NoError(self.T(), err)

		written = append(written, message)
	}

	// Wait until all messages are visible
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		tasks, err := client_info_manager.PeekClientTasks(
			context.Background(), self.client_id)
		assert.NoError(self.T(), err)
		return 10 == len(tasks)
	})

	tasks, err := client_info_manager.GetClientTasks(
		context.Background(), self.client_id)
	assert.NoError(self.T(), err)

	// Does not have to return in sorted form.
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].SessionId < tasks[j].SessionId
	})

	for i := 0; i < 10; i++ {
		assert.True(self.T(), proto.Equal(tasks[i], written[i]))
	}
}

func (self *ClientInfoTestSuite) TestInFlightMessages() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 0)))
	defer closer()

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	var flow_ids []string
	acl_manager := acl_managers.NullACLManager{}
	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)

	for i := 0; i < 10; i++ {
		closer := utils.SetFlowIdForTests(fmt.Sprintf("F.%d", i))

		flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx,
			self.ConfigObj, acl_manager,
			repository, &flows_proto.ArtifactCollectorArgs{
				Creator:   "admin",
				ClientId:  self.client_id,
				Artifacts: []string{"Client.Test"},
			}, utils.SyncCompleter)
		assert.NoError(self.T(), err)

		flow_ids = append(flow_ids, flow_id)

		closer()
	}

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	tasks, err := client_info_manager.GetClientTasks(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)

	// 4 tasks are queued
	assert.Equal(self.T(), len(tasks), 4)

	tasks, err = client_info_manager.GetClientTasks(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)

	// Tasks are still in flight, so we can not get any new tasks yet.
	assert.Equal(self.T(), len(tasks), 0)

	client_info, err := client_info_manager.Get(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)

	// Should contain only 4 flow ids in the in_flight_flows set.
	golden := ordereddict.NewDict().
		Set("InFlightFlows", client_info)

	// Pass some time
	closer = utils.MockTime(utils.NewMockClock(time.Unix(100, 0)))
	defer closer()

	// Tasks are still in flight, so we do not send any flows, instead
	// we send a task status request to see how those other tasks are
	// going.
	tasks, err = client_info_manager.GetClientTasks(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)

	sort.Strings(tasks[0].FlowStatsRequest.FlowId)

	assert.Equal(self.T(), len(tasks), 1)

	// Should contains a status check request for all inflight flows.
	golden.Set("StatusChecks", tasks)

	// Now complete the flows
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	for flow_id := range client_info.InFlightFlows {
		runner.ProcessSingleMessage(self.Ctx,
			&crypto_proto.VeloMessage{
				Source:    self.client_id,
				SessionId: flow_id,
				FlowStats: &crypto_proto.FlowStats{
					FlowComplete: true,
					QueryStatus: []*crypto_proto.VeloStatus{{
						Status: crypto_proto.VeloStatus_OK,
					}},
				}})
	}
	runner.Close(self.Ctx)

	client_info, err = client_info_manager.Get(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)

	// Completing the flows removes the flows from the in flight set.
	assert.Equal(self.T(), len(client_info.InFlightFlows), 0)

	// Should contain no in flight flows but still contain the
	// has_tasks flag.
	golden.Set("AfterCompletion", client_info)

	// Now read some more tasks
	tasks, err = client_info_manager.GetClientTasks(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)

	// Should conatin
	assert.Equal(self.T(), len(tasks), 4)

	client_info, err = client_info_manager.Get(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)
	golden.Set("SecondSetOfTasks", client_info)

	goldie.Assert(self.T(), "TestInFlightMessages", json.MustMarshalIndent(golden))
}
