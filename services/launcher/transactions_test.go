package launcher_test

import (
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *LauncherTestSuite) TestTransactions() {
	t := self.T()

	flow_id := "F.FlowId123"
	client_id := "C.1212"
	user := "admin"

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(t, err)

	// Create a new client
	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{ClientId: client_id},
	})
	assert.NoError(t, err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(t, err)

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)
	acl_manager := acl_managers.NullACLManager{}

	defer utils.SetFlowIdForTests(flow_id)()

	// Schedule a job for the server runner.
	flow_id, err = launcher.ScheduleArtifactCollection(
		self.Ctx, self.ConfigObj, acl_manager,
		repository, &flows_proto.ArtifactCollectorArgs{
			Creator:   user,
			ClientId:  client_id,
			Artifacts: []string{"Generic.Client.Info"},
		}, utils.SyncCompleter)
	assert.NoError(t, err)

	// Drain the tasks from the queue
	tasks, err := client_info_manager.GetClientTasks(self.Ctx, client_id)
	assert.NoError(t, err)
	assert.Equal(self.T(), len(tasks), 2)

	// Emulate a client message that an upload is scheduled. These
	// messages are the mirror image from actions/transactions_test.go
	flow_runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	flow_runner.ProcessSingleMessage(self.Ctx, &crypto_proto.VeloMessage{
		SessionId: flow_id,
		Source:    client_id,
		UploadTransaction: &actions_proto.UploadTransaction{
			Filename:   "/bin/ls",
			Components: []string{"bin", "ls"},
		},
	})

	// The client reports the number of transactions sent in the FlowStats
	flow_runner.ProcessSingleMessage(self.Ctx, &crypto_proto.VeloMessage{
		SessionId: flow_id,
		Source:    client_id,
		FlowStats: &crypto_proto.FlowStats{
			TransactionsOutstanding: 1,

			// Emulate the flow is errored - e.g. it timed out.
			QueryStatus: []*crypto_proto.VeloStatus{{
				Status:            crypto_proto.VeloStatus_GENERIC_ERROR,
				ErrorMessage:      "Timeed out",
				NamesWithResponse: []string{"Generic.Client.Info"},
			}},
		},
	})
	flow_runner.Close(self.Ctx)

	flow_context, err := launcher.Storage().LoadCollectionContext(
		self.Ctx, self.ConfigObj, client_id, flow_id)
	assert.NoError(t, err)

	// The collection context reports the total number of transactions
	// outstanding.
	assert.Equal(self.T(), flow_context.TransactionsOutstanding, uint64(1))

	// The entire flow is marked in error.
	assert.Equal(self.T(), flow_context.State,
		flows_proto.ArtifactCollectorContext_ERROR)

	// There is a single query (according to the FlowStats message above).
	assert.Equal(self.T(), len(flow_context.QueryStats), 1)

	// Now we are about to resume the flow.
	transactions, err := launcher.ResumeFlow(self.Ctx, self.ConfigObj,
		client_id, flow_id)
	assert.NoError(t, err)

	assert.Equal(self.T(), len(transactions), 1)

	// Check the collection
	flow_context, err = launcher.Storage().LoadCollectionContext(
		self.Ctx, self.ConfigObj, client_id, flow_id)
	assert.NoError(t, err)

	// collection is moved into the scheduled state now. This means it
	// is scheduled but not yet running on the client.
	assert.Equal(self.T(), flow_context.State,
		flows_proto.ArtifactCollectorContext_IN_PROGRESS)

	// There are now two queries
	assert.Equal(self.T(), len(flow_context.QueryStats), 2)

	// The second query is for the virtual artifact
	assert.Equal(self.T(), flow_context.QueryStats[1].NamesWithResponse[0],
		constants.UPLOAD_RESUMED_SOURCE)

	// Start time is set
	assert.True(self.T(), flow_context.QueryStats[1].FirstActive > 0)

	// Now check the requests scheduled for the client.
	tasks, err = client_info_manager.GetClientTasks(self.Ctx, client_id)
	assert.NoError(t, err)

	assert.Equal(self.T(), len(tasks), 1)

	// A special ResumeTransactions message
	assert.NotNil(self.T(), tasks[0].ResumeTransactions)

	// Contains one transaction
	assert.Equal(self.T(), len(tasks[0].ResumeTransactions.Transactions), 1)
	assert.Equal(self.T(),
		tasks[0].ResumeTransactions.Transactions[0].Filename, "/bin/ls")

	// Contains two Queries
	assert.Equal(self.T(), len(tasks[0].ResumeTransactions.QueryStats), 2)

	// The first query state is set to completed.
	assert.Equal(self.T(),
		tasks[0].ResumeTransactions.QueryStats[0].NamesWithResponse[0],
		"Generic.Client.Info")

	assert.Equal(self.T(),
		tasks[0].ResumeTransactions.QueryStats[0].Status,
		crypto_proto.VeloStatus_OK)

	// The second query state is set to in progress for the Resumption query.
	assert.Equal(self.T(),
		tasks[0].ResumeTransactions.QueryStats[1].NamesWithResponse[0],
		constants.UPLOAD_RESUMED_SOURCE)

	assert.Equal(self.T(),
		tasks[0].ResumeTransactions.QueryStats[1].Status,
		crypto_proto.VeloStatus_PROGRESS)

}
