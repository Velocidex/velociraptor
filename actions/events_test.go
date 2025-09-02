package actions_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

var (
	artifact_definitions = []string{`
name: EventArtifact1
type: CLIENT_EVENT
parameters:
- name: Foo
sources:
- query: SELECT * FROM info()
`, `
name: EventArtifact2
type: CLIENT_EVENT
sources:
- query: SELECT * FROM info()
`}
)

type EventsTestSuite struct {
	test_utils.TestSuite
	client_id string
	responder *responder.TestResponderType
	writeback string

	event_table *actions.EventTable

	closer func()
}

func (self *EventsTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.LoadArtifactsIntoConfig(artifact_definitions)

	// Set a tempfile for the writeback we need to check that the
	// new event query is written there.
	tmpfile, err := tempfile.TempFile("")
	assert.NoError(self.T(), err)
	tmpfile.Close()

	self.writeback = tmpfile.Name()
	self.ConfigObj.Client.WritebackLinux = self.writeback
	self.ConfigObj.Client.WritebackWindows = self.writeback
	self.ConfigObj.Client.WritebackDarwin = self.writeback
	self.ConfigObj.Services.ClientMonitoring = true
	self.ConfigObj.Services.IndexServer = true

	datastore.SetGlobalDatastore(context.Background(),
		self.ConfigObj.Datastore.Implementation, self.ConfigObj)

	self.TestSuite.SetupTest()

	writeback_service := writeback.GetWritebackService()
	writeback_service.LoadWriteback(self.ConfigObj)

	self.client_id = "C.2232"
	self.closer = utils.MockTime(&utils.IncClock{})

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: self.client_id,
		},
	})

	self.responder = responder.TestResponderWithFlowId(
		self.ConfigObj, "EventsTestSuite")
	self.event_table = actions.NewEventTable(
		self.Ctx, self.Wg, self.ConfigObj)
	self.event_table.UpdateEventTable(
		self.Ctx, self.Wg, self.ConfigObj,
		self.responder.Output(),
		&actions_proto.VQLEventTable{})
}

func (self *EventsTestSuite) InitializeEventTable(ctx context.Context,
	wg *sync.WaitGroup, output_chan chan *crypto_proto.VeloMessage) *actions.EventTable {
	result := actions.NewEventTable(ctx, wg, self.ConfigObj)
	result.UpdateEventTable(ctx, wg, self.ConfigObj,
		output_chan, &actions_proto.VQLEventTable{})

	return result
}

func (self *EventsTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()

	if self.closer != nil {
		self.closer()
	}

	os.Remove(self.writeback) // clean up file buffer
}

func server_state() *flows_proto.ClientEventTable {
	return &flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			// These apply to all labels.
			Artifacts: []string{"EventArtifact1"},
		},

		// If the client is labeled as "Label1" then it will
		// receive these
		LabelEvents: []*flows_proto.LabelEvents{{
			Label: "Label1",
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"EventArtifact2"},
			}},
		},
	}
}

func (self *EventsTestSuite) TestEventTableUpdate() {
	client_manager, err := services.ClientEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithTimeout(self.Ctx, time.Second*60)
	defer cancel()

	// Wait until the entire event table is cleaned up.
	output_chan, _ := responder.NewMessageDrain(ctx)
	table := self.InitializeEventTable(ctx, wg, output_chan)

	require.NoError(self.T(), client_manager.SetClientMonitoringState(
		ctx, self.ConfigObj, "", server_state()))

	// Check the version of the initial Event table it should be 0
	version := table.Version()
	assert.Equal(self.T(), uint64(0), version)

	// We definitely need to update the table on this client.
	assert.True(self.T(),
		client_manager.CheckClientEventsVersion(
			self.Ctx,
			self.ConfigObj, self.client_id, version))

	// Get the new table
	message := client_manager.GetClientUpdateEventTableMessage(
		self.Ctx, self.ConfigObj, self.client_id)

	// Only one query will be selected now since no label is set
	// on the client.
	assert.Equal(self.T(), len(message.UpdateEventTable.Event), 1)
	assert.Equal(self.T(), utils.GetQueryName(
		message.UpdateEventTable.Event[0].Query), "EventArtifact1")

	// Set the new table, this will execute the new queries and
	// start the new table.
	actions.QueryLog.Clear()
	table.UpdateEventTable(ctx, wg, self.ConfigObj, output_chan,
		message.UpdateEventTable)

	// Table version was upgraded
	version = table.Version()
	assert.NotEqual(self.T(), version, 0)

	// And we ran some queries.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return len(actions.QueryLog.Get()) > 1
	})
	actions.QueryLog.Clear()

	// We no longer need to update the event table - it is up to date.
	assert.False(self.T(),
		client_manager.CheckClientEventsVersion(
			self.Ctx, self.ConfigObj, self.client_id,
			table.Version()))

	// Now we set a label on the client. This should cause the
	// event table to be recalculated but since the label does not
	// actually change the label groups, the new event table will
	// be the same as the old one, except the version will be
	// advanced.
	label_manager := services.GetLabeler(self.ConfigObj)

	require.NoError(self.T(),
		label_manager.SetClientLabel(
			self.Ctx, self.ConfigObj, self.client_id, "Foobar"))

	// Setting the label will cause the client_monitoring manager
	// to want to upgrade the event table.
	assert.True(self.T(),
		client_manager.CheckClientEventsVersion(
			self.Ctx, self.ConfigObj, self.client_id,
			table.Version()))

	new_message := client_manager.GetClientUpdateEventTableMessage(
		self.Ctx, self.ConfigObj, self.client_id)

	assert.True(self.T(), new_message.UpdateEventTable.Version >
		message.UpdateEventTable.Version)

	// The new table has 1 queries still since it has not really changed.
	assert.Equal(self.T(), len(new_message.UpdateEventTable.Event), 1)

	// Now check that no updates are performed: We clear the query log
	// and send an update. No new queries should be running.
	actions.QueryLog.Clear()

	table.UpdateEventTable(ctx, wg, self.ConfigObj, output_chan,
		new_message.UpdateEventTable)

	// Wait for the event table version to change
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return version != table.Version()
	})

	// But the tables have not really changed, so the query will
	// not be updated.
	queries := actions.QueryLog.Get()
	if len(queries) != 0 {
		fmt.Printf("Queries that ran %v\n", queries)
	}
	assert.Equal(self.T(), len(queries), 0)

	// Now lets set the label to Label1
	require.NoError(self.T(),
		label_manager.SetClientLabel(
			self.Ctx, self.ConfigObj,
			self.client_id, "Label1"))

	// We need to update the table again (takes a while for the
	// client manager to notice the label change).
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return client_manager.CheckClientEventsVersion(
			self.Ctx, self.ConfigObj, self.client_id,
			table.Version())
	})

	new_message = client_manager.GetClientUpdateEventTableMessage(
		self.Ctx, self.ConfigObj, self.client_id)

	// The new table has 2 event queries - one for the All label
	// and one for Label1 label.
	assert.Equal(self.T(), len(new_message.UpdateEventTable.Event), 2)

	table.UpdateEventTable(ctx, wg, self.ConfigObj, output_chan,
		new_message.UpdateEventTable)

	// Wait for the event table to be swapped.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return len(actions.QueryLog.Get()) > 2
	})

	// At least 2 queries were run
	assert.True(self.T(), len(actions.QueryLog.Get()) > 2)

	fd, err := os.Open(self.writeback)
	assert.NoError(self.T(), err)
	data, err := ioutil.ReadAll(fd)
	assert.NoError(self.T(), err)

	// Make sure the event queries end up in the writeback file
	assert.Contains(self.T(), string(data), "EventArtifact1")
	assert.Contains(self.T(), string(data), "EventArtifact2")

	// The below checks that the event table is updated if only a
	// parameter is changed.

	// Check that Foo is empty right now
	assert.Equal(self.T(), "", table.Events[0].Env[0].Value)

	// Update the monitoring table but only change artifact
	// parameters. Set Foo to "X"
	new_state := server_state()
	new_state.Artifacts.Specs = append(new_state.Artifacts.Specs,
		&flows_proto.ArtifactSpec{
			Artifact: "EventArtifact1",
			Parameters: &flows_proto.ArtifactParameters{
				Env: []*actions_proto.VQLEnv{
					{Key: "Foo", Value: "X"},
				},
			},
		})

	require.NoError(self.T(), client_manager.SetClientMonitoringState(
		ctx, self.ConfigObj, "", new_state))

	new_message = client_manager.GetClientUpdateEventTableMessage(
		self.Ctx, self.ConfigObj, self.client_id)

	// Force the update on the table.
	table.UpdateEventTable(ctx, wg, self.ConfigObj, output_chan,
		new_message.UpdateEventTable)

	// The update took hold - the new parameter value is "X"
	assert.Equal(self.T(), "X", table.Events[0].Env[0].Value)
}

// What do we consider a change in the event table. The server may
// send updated event tables frequently but we do not want to
// interrupt the event tables if the queries do not really
// change. This checks we skip the table update if it is the same as
// before.
func (self *EventsTestSuite) TestEventEqual() {
	client_manager, err := services.ClientEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	ctx, cancel := context.WithTimeout(self.Ctx, time.Second*60)
	defer cancel()

	// Wait until the entire event table is cleaned up.
	wg := &sync.WaitGroup{}
	output_chan, _ := responder.NewMessageDrain(ctx)
	table := self.InitializeEventTable(ctx, wg, output_chan)
	_ = table

	require.NoError(self.T(), client_manager.SetClientMonitoringState(
		ctx, self.ConfigObj, "", server_state()))

	message := client_manager.GetClientUpdateEventTableMessage(
		self.Ctx, self.ConfigObj, self.client_id)

	// Update the table for the base message.
	err, ok := table.Update(ctx, wg, self.ConfigObj, output_chan,
		message.UpdateEventTable)
	assert.NoError(self.T(), err)
	assert.True(self.T(), ok)

	// Now we try check if the table will update under certain conditions.

	// Increase the version but no difference in content at all
	message.UpdateEventTable.Version += 100
	err, ok = table.Update(ctx, wg, self.ConfigObj, output_chan,
		message.UpdateEventTable)
	assert.NoError(self.T(), err)
	assert.False(self.T(), ok)

	// A query was added to the table
	message.UpdateEventTable.Version += 100
	message.UpdateEventTable.Event[0].Query = append(
		message.UpdateEventTable.Event[0].Query,
		&actions_proto.VQLRequest{
			VQL: "SELECT * FROM info()",
		})

	err, ok = table.Update(ctx, wg, self.ConfigObj, output_chan,
		message.UpdateEventTable)
	assert.NoError(self.T(), err)

	// Yes this is a new query!
	assert.True(self.T(), ok)

	// Now add a new parameter - this is also an update
	message.UpdateEventTable.Version += 100
	message.UpdateEventTable.Event[0].Env = append(
		message.UpdateEventTable.Event[0].Env, &actions_proto.VQLEnv{
			Key:   "Foo",
			Value: "Bar",
		})

	err, ok = table.Update(ctx, wg, self.ConfigObj, output_chan,
		message.UpdateEventTable)
	assert.NoError(self.T(), err)

	// Yes this is a new query!
	assert.True(self.T(), ok)

	// Change the parameter
	message.UpdateEventTable.Version += 100
	message.UpdateEventTable.Event[0].Env[0].Value = "Baz"

	err, ok = table.Update(ctx, wg, self.ConfigObj, output_chan,
		message.UpdateEventTable)
	assert.NoError(self.T(), err)

	// Yes this is a new query!
	assert.True(self.T(), ok)
}

func TestEventsTestSuite(t *testing.T) {
	suite.Run(t, &EventsTestSuite{})
}
