package actions_test

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_monitoring"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/labels"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

var (
	artifact_definitions = []string{`
name: EventArtifact1
type: CLIENT_EVENT
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
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	sm         *services.Service
	responder  *responder.Responder
	writeback  string
}

func (self *EventsTestSuite) SetupTest() {
	t := self.T()

	self.client_id = "C.2232"
	self.config_obj = config.GetDefaultConfig()
	self.config_obj.Frontend.DoNotCompressArtifacts = true
	self.config_obj.Datastore.Implementation = "Test"

	tmpfile, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	tmpfile.Close()

	// Set a tempfile for the writeback we need to check that the
	// new event query is written there.
	self.writeback = tmpfile.Name()
	self.config_obj.Client.WritebackLinux = self.writeback
	self.config_obj.Client.WritebackWindows = self.writeback
	self.config_obj.Client.WritebackDarwin = self.writeback

	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	// Start the journaling service manually for tests.
	require.NoError(t, self.sm.Start(journal.StartJournalService))
	require.NoError(t, self.sm.Start(launcher.StartLauncherService))
	require.NoError(t, self.sm.Start(labels.StartLabelService))
	require.NoError(t, self.sm.Start(notifications.StartNotificationService))
	require.NoError(t, self.sm.Start(inventory.StartInventoryService))
	require.NoError(t, self.sm.Start(repository.StartRepositoryManager))
	require.NoError(t, self.sm.Start(client_monitoring.StartClientMonitoringService))
	self.responder = responder.TestResponder()

	actions.GlobalEventTable = actions.NewEventTable(
		self.config_obj, self.responder,
		&actions_proto.VQLEventTable{})

	manager, err := services.GetRepositoryManager()
	assert.NoError(t, err)
	repository, err := manager.GetGlobalRepository(self.config_obj)
	assert.NoError(t, err)

	for _, definition := range artifact_definitions {
		_, err := repository.LoadYaml(definition, true)
		require.NoError(t, err)
	}
}

func (self *EventsTestSuite) TearDownTest() {
	self.sm.Close()

	os.Remove(self.writeback) // clean up file buffer
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
}

var server_state = &flows_proto.ClientEventTable{
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

func (self *EventsTestSuite) TestEventTableUpdate() {
	client_manager := services.ClientEventManager()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	require.NoError(self.T(), client_manager.SetClientMonitoringState(
		ctx, self.config_obj, server_state))

	// Check the version of the initial Event table it should be 0
	version := actions.GlobalEventTableVersion()
	assert.Equal(self.T(), uint64(0), version)

	// We definitely need to update the table on this client.
	assert.True(self.T(),
		client_manager.CheckClientEventsVersion(
			self.config_obj, self.client_id, version))

	// Get the new table
	message := client_manager.GetClientUpdateEventTableMessage(
		self.config_obj, self.client_id)

	// Only one query will be selected now since no label is set
	// on the client.
	assert.Equal(self.T(), len(message.UpdateEventTable.Event), 1)
	assert.Equal(self.T(), getQueryName(message.UpdateEventTable.Event[0]),
		"EventArtifact1")

	// Set the new table, this will execute the new queries and
	// start the new table.
	actions.QueryLog.Clear()
	actions.UpdateEventTable{}.Run(self.config_obj, ctx, self.responder,
		message.UpdateEventTable)

	// Table version was upgraded
	version = actions.GlobalEventTableVersion()
	assert.NotEqual(self.T(), version, 0)

	// And we ran some queries.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return len(actions.QueryLog.Get()) > 0
	})

	// We no longer need to update the event table - it is up to date.
	assert.False(self.T(),
		client_manager.CheckClientEventsVersion(
			self.config_obj, self.client_id,
			actions.GlobalEventTableVersion()))

	// Now we set a label on the client. This should cause the
	// event table to be recalculated but since the label does not
	// actually change the label groups, the new event table will
	// be the same as the old one, except the version will be
	// advanced.
	label_manager := services.GetLabeler()
	require.NoError(self.T(),
		label_manager.SetClientLabel(self.config_obj, self.client_id,
			"Foobar"))

	// Setting the label will cause the client_monitoring manager
	// to want to upgrade the event table.
	assert.True(self.T(),
		client_manager.CheckClientEventsVersion(
			self.config_obj, self.client_id,
			actions.GlobalEventTableVersion()))

	new_message := client_manager.GetClientUpdateEventTableMessage(
		self.config_obj, self.client_id)

	assert.True(self.T(), new_message.UpdateEventTable.Version >
		message.UpdateEventTable.Version)

	// The new table has 1 queries still since it has not really changed.
	assert.Equal(self.T(), len(new_message.UpdateEventTable.Event), 1)

	// Lets update the event table with the new version.
	actions.QueryLog.Clear()
	actions.UpdateEventTable{}.Run(self.config_obj, ctx, self.responder,
		new_message.UpdateEventTable)

	// Wait for the event table version to change
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return version != actions.GlobalEventTableVersion()
	})

	// But the tables have not really changed, so the query will
	// not be updated.
	assert.Equal(self.T(), len(actions.QueryLog.Get()), 0)

	// Now lets set the label to Label1
	require.NoError(self.T(),
		label_manager.SetClientLabel(self.config_obj, self.client_id,
			"Label1"))

	// We need to update the table again (takes a while for the
	// client manager to notice the label change).
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return client_manager.CheckClientEventsVersion(
			self.config_obj, self.client_id,
			actions.GlobalEventTableVersion())
	})

	new_message = client_manager.GetClientUpdateEventTableMessage(
		self.config_obj, self.client_id)

	// The new table has 2 event queries - one for the All label
	// and one for Label1 label.
	assert.Equal(self.T(), len(new_message.UpdateEventTable.Event), 2)

	actions.UpdateEventTable{}.Run(self.config_obj, ctx, self.responder,
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
}

func getQueryName(args *actions_proto.VQLCollectorArgs) string {
	for _, query := range args.Query {
		if query.Name != "" {
			return query.Name
		}
	}
	return ""
}

func TestEventsTestSuite(t *testing.T) {
	suite.Run(t, &EventsTestSuite{})
}
