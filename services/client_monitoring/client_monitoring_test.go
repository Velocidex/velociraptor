package client_monitoring

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/labels"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type ClientMonitoringTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	flow_id    string
	sm         *services.Service
}

func (self *ClientMonitoringTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	// We want to see the artifacts name plainly
	self.config_obj.Frontend.DoNotCompressArtifacts = true

	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	// Start the journaling service manually for tests.
	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(inventory.StartInventoryService))
	require.NoError(self.T(), self.sm.Start(labels.StartLabelService))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
	require.NoError(self.T(), self.sm.Start(StartClientMonitoringService))

	self.client_id = "C.12312"
}

func (self *ClientMonitoringTestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

// Check that monitoring tables eventually follow when artifact
// definitions are updated.
func (self *ClientMonitoringTestSuite) TestUpdatingArtifacts() {
	current_clock := &utils.IncClock{NowTime: 10}

	repository_manager := services.GetRepositoryManager()
	repository_manager.SetArtifactFile(`
name: TestArtifact
sources:
- query:
    SELECT * FROM info()
`, "")

	manager := services.ClientEventManager().(*ClientEventTable)
	manager.clock = current_clock

	err := manager.SetClientMonitoringState(&flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
		},
	})
	assert.NoError(self.T(), err)

	old_table := manager.GetClientUpdateEventTableMessage(self.client_id)
	assert.NotContains(self.T(), json.StringIndent(old_table), "Crib")

	// Now update the artifact.
	repository_manager.SetArtifactFile(`
name: TestArtifact
sources:
- query:
    SELECT *, Crib FROM info()
`, "")

	// The table should magically be updated!
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		table := manager.GetClientUpdateEventTableMessage(self.client_id)
		return strings.Contains(json.StringIndent(table), "Crib")
	})
}

func (self *ClientMonitoringTestSuite) TestUpdatingClientTable() {
	current_clock := &utils.IncClock{NowTime: 10}

	repository_manager := services.GetRepositoryManager()
	repository_manager.SetArtifactFile(`
name: TestArtifact
sources:
- query:
    SELECT * FROM info()
`, "")

	manager := services.ClientEventManager().(*ClientEventTable)
	manager.clock = current_clock

	// Set the initial table.
	err := manager.SetClientMonitoringState(&flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
		},
	})
	assert.NoError(self.T(), err)

	// Get the client's event table
	old_table := manager.GetClientUpdateEventTableMessage(self.client_id)

	// Now update the monitoring state
	err = manager.SetClientMonitoringState(&flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
		},
	})
	assert.NoError(self.T(), err)

	// Get the client event table again
	table := manager.GetClientUpdateEventTableMessage(self.client_id)

	// Make sure the client event table version is updated.
	assert.True(self.T(),
		table.UpdateEventTable.Version > old_table.UpdateEventTable.Version)
}

func (self *ClientMonitoringTestSuite) TestUpdatingClientTableMultiFrontend() {
	current_clock := &utils.IncClock{NowTime: 10}

	repository_manager := services.GetRepositoryManager()
	repository_manager.SetArtifactFile(`
name: TestArtifact
sources:
- query:
    SELECT * FROM info()
`, "")

	manager1 := services.ClientEventManager().(*ClientEventTable)
	manager1.clock = current_clock

	// Set the initial table.
	err := manager1.SetClientMonitoringState(&flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
		},
	})
	assert.NoError(self.T(), err)

	// Get the client's event table
	old_table := manager1.GetClientUpdateEventTableMessage(self.client_id)

	// Now another frontend sets the client monitoring state
	require.NoError(self.T(), self.sm.Start(StartClientMonitoringService))
	manager2 := services.ClientEventManager().(*ClientEventTable)
	manager2.clock = current_clock

	// Now update the monitoring state
	err = manager2.SetClientMonitoringState(&flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
		},
	})
	assert.NoError(self.T(), err)

	// Get the client event table again
	table := manager2.GetClientUpdateEventTableMessage(self.client_id)

	// Make sure the client event table version is updated.
	assert.True(self.T(),
		table.UpdateEventTable.Version > old_table.UpdateEventTable.Version)
}

func (self *ClientMonitoringTestSuite) TestClientMonitoringCompiling() {
	// Every time the clock gives time.Now() it is forced to
	// increment.
	current_clock := &utils.IncClock{NowTime: 10}

	labeler := services.GetLabeler().(*labels.Labeler)
	labeler.Clock = current_clock

	// If no table exists, we will get a default table.
	manager := services.ClientEventManager().(*ClientEventTable)
	manager.clock = current_clock

	// Install an initial monitoring table: Everyone gets ServiceCreation.
	manager.SetClientMonitoringState(&flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Windows.Events.ServiceCreation"},
		},
	})

	table := manager.GetClientUpdateEventTableMessage(self.client_id)

	// There should be one event table sent.
	assert.Equal(self.T(), len(table.UpdateEventTable.Event), 1)

	version := table.UpdateEventTable.Version
	assert.True(self.T(), version > 0)

	// Now the client upgraded its table, do we need to update it again?
	assert.False(self.T(), manager.CheckClientEventsVersion(self.client_id, version))

	// Add a label to the client
	require.NoError(self.T(), labeler.SetClientLabel(self.client_id, "Label1"))

	// Since the client's label changed it might need to be updated.
	assert.True(self.T(), manager.CheckClientEventsVersion(self.client_id, version))

	// But the event table does not include a rule for this label anyway.
	table = manager.GetClientUpdateEventTableMessage(self.client_id)
	assert.Equal(self.T(), len(table.UpdateEventTable.Event), 1)

	// New table is still updated though.
	assert.True(self.T(), version < table.UpdateEventTable.Version)
	version = table.UpdateEventTable.Version

	// Now lets install a new label rule for this label and another label.
	manager.SetClientMonitoringState(&flows_proto.ClientEventTable{
		// All clients should have ServiceCreation
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Windows.Events.ServiceCreation"},
		},
		LabelEvents: []*flows_proto.LabelEvents{
			// DNS Queries for Label1
			{Label: "Label1", Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"Windows.Events.DNSQueries"},
			}},

			// ProcessCreation for Label2
			{Label: "Label2", Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"Windows.Events.ProcessCreation"},
			}},
		},
	})

	// A new table is installed, this client must update.
	assert.True(self.T(), manager.CheckClientEventsVersion(self.client_id, version))

	// The new table includes 2 rules - the default and for Label1
	table = manager.GetClientUpdateEventTableMessage(self.client_id)
	assert.Equal(self.T(), len(table.UpdateEventTable.Event), 2)

	// New table has a later version.
	assert.True(self.T(), version < table.UpdateEventTable.Version)
	version = table.UpdateEventTable.Version

	// The client only has Label1 set so should only receive ServiceCreation and DNSQueries
	assert.Equal(self.T(), extractArtifacts(table.UpdateEventTable),
		[]string{"Windows.Events.ServiceCreation", "Windows.Events.DNSQueries"})

	// Lets add Label2 to this client.
	labeler.SetClientLabel(self.client_id, "Label2")

	// A new table is installed, this client must update.
	assert.True(self.T(), manager.CheckClientEventsVersion(self.client_id, version))

	// The new table includes 3 rules - the default and for Label1 and Label2
	table = manager.GetClientUpdateEventTableMessage(self.client_id)
	assert.Equal(self.T(), len(table.UpdateEventTable.Event), 3)

	// New table has a later version.
	assert.True(self.T(), version < table.UpdateEventTable.Version)
	version = table.UpdateEventTable.Version

	// The client should now receive all events
	assert.Equal(self.T(), extractArtifacts(table.UpdateEventTable),
		[]string{"Windows.Events.ServiceCreation",
			"Windows.Events.DNSQueries",
			"Windows.Events.ProcessCreation"})

	// We are done now... no need to update anymore.
	assert.False(self.T(), manager.CheckClientEventsVersion(self.client_id, version))
}

// Event queries are asyncronous and blocking so when collecting
// multiple queries, we need to send each query in its own Event entry
// so they can run in parallel. The client runs each Event object in a
// separate goroutine. It is not allowed to send multiple SELECT
// statements in the same event because this will block on the first
// SELECT and never reach the second SELECT. This test checks for this
// condition.
func (self *ClientMonitoringTestSuite) TestClientMonitoringCompilingMultipleArtifacts() {
	current_clock := &utils.IncClock{NowTime: 10}

	labeler := services.GetLabeler().(*labels.Labeler)
	labeler.Clock = current_clock

	// If no table exists, we will get a default table.
	manager := services.ClientEventManager().(*ClientEventTable)
	manager.clock = current_clock

	// Install an initial monitoring table: Everyone gets ServiceCreation.
	manager.SetClientMonitoringState(&flows_proto.ClientEventTable{
		Artifacts: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{
				"Windows.Events.ServiceCreation",
				"Windows.Events.ProcessCreation",
			},
		},
	})
	table := manager.GetClientUpdateEventTableMessage(self.client_id)

	// Count how many SELECT statements exist in each event table.
	for _, event := range table.UpdateEventTable.Event {
		count := 0
		for _, query := range event.Query {
			if strings.HasPrefix(query.VQL, "SELECT") {
				count++
			}
		}
		assert.Equal(self.T(), 1, count)
	}
}

func extractArtifacts(args *actions_proto.VQLEventTable) []string {
	result := []string{}

	for _, event := range args.Event {
		for _, query := range event.Query {
			if query.Name != "" {
				result = append(result, query.Name)
			}
		}
	}

	return result
}

// Check that labels are properly populated from the index.
func (self *ClientMonitoringTestSuite) TestClientMonitoring() {
	current_clock := &utils.MockClock{MockNow: time.Unix(10, 0)}

	labeler := services.GetLabeler().(*labels.Labeler)
	labeler.Clock = current_clock

	// If no table exists, we will get a default table.
	manager := services.ClientEventManager().(*ClientEventTable)
	manager.clock = current_clock

	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
	assert.NoError(self.T(), manager.LoadFromFile())

	table := manager.GetClientMonitoringState()

	// Version is based on timestamp.
	assert.Equal(self.T(), table.Version, uint64(10000000000))
	assert.Equal(self.T(), table.Artifacts.Artifacts, []string{"Generic.Client.Stats"})

	// If a client presents an earlier version table they will
	// need to update.
	assert.True(self.T(), manager.CheckClientEventsVersion(
		self.client_id, 50))

	// If a client presents the same table version they dont need to do anything.
	assert.False(self.T(), manager.CheckClientEventsVersion(
		self.client_id, uint64(10000000000)))

	// Some time later we label the client.
	current_clock.MockNow = time.Unix(20, 0)
	labeler.SetClientLabel(self.client_id, "Foobar")

	// Client will now be required to update its event table to
	// make sure the new label does not apply.
	assert.True(self.T(), manager.CheckClientEventsVersion(
		self.client_id, uint64(10000000000)))

}

func TestClientMonitoringService(t *testing.T) {
	suite.Run(t, &ClientMonitoringTestSuite{})
}
