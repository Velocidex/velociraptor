package client_monitoring_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_monitoring"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

var (
	mock_definitions = []string{`
name: Windows.Remediation.QuarantineMonitor
type: CLIENT_EVENT
`, `
name: Server.Internal.Label
type: INTERNAL
`, `
name: Windows.Events.ProcessCreation
type: CLIENT_EVENT
sources:
- precondition: SELECT OS from info() where OS = "windows"
  query: SELECT * FROM info()
`, `
name: Windows.Events.DNSQueries
type: CLIENT_EVENT
sources:
- precondition: SELECT OS from info() where OS = "windows"
  query: SELECT * FROM info()
`, `
name: Windows.Events.ServiceCreation
type: CLIENT_EVENT
sources:
- precondition: SELECT OS from info() where OS = "windows"
  query: SELECT * FROM info()
`,
	}
)

type ClientMonitoringTestSuite struct {
	test_utils.TestSuite
	client_id string
	flow_id   string
}

func (self *ClientMonitoringTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.ClientMonitoring = true

	self.LoadArtifactsIntoConfig(mock_definitions)

	self.TestSuite.SetupTest()
	self.client_id = "C.12312"

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	_, err = repository.LoadYaml(`
name: TestArtifact
sources:
- query:
    SELECT * FROM info()
`, services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: false})

	assert.NoError(self.T(), err)
	_, err = repository.LoadYaml(`
name: SomethingElse
sources:
- query:
    SELECT * FROM info()
`, services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: false})

	assert.NoError(self.T(), err)

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: self.client_id,
		},
	})
}

// Check that monitoring tables eventually follow when artifact
// definitions are updated.
func (self *ClientMonitoringTestSuite) TestUpdatingArtifacts() {
	closer := utils.MockTime(&utils.IncClock{NowTime: 10})
	defer closer()

	manager, err := services.ClientEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = manager.SetClientMonitoringState(
		context.Background(), self.ConfigObj, "",
		&flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"TestArtifact", "SomethingElse"},
			},
		})
	assert.NoError(self.T(), err)

	old_table_message := manager.GetClientUpdateEventTableMessage(
		context.Background(), self.ConfigObj, self.client_id)
	assert.NotContains(self.T(), json.StringIndent(old_table_message), "Crib")

	table_version := old_table_message.UpdateEventTable.Version

	// Now update the artifact.
	repository_manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	ctx := self.Ctx
	_, err = repository_manager.SetArtifactFile(ctx,
		self.ConfigObj, "", `
name: TestArtifact
sources:
- query:
    SELECT *, Crib FROM info()
`, "")
	require.NoError(self.T(), err)

	var new_table_message *crypto_proto.VeloMessage

	// The table should magically be updated!
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		if !manager.CheckClientEventsVersion(
			context.Background(), self.ConfigObj, self.client_id, table_version) {
			return false
		}

		new_table_message = manager.GetClientUpdateEventTableMessage(
			context.Background(), self.ConfigObj, self.client_id)
		return strings.Contains(json.StringIndent(new_table_message), "Crib")
	})

	// Make sure the table version is updated
	assert.True(self.T(),
		table_version < new_table_message.UpdateEventTable.Version)

	table_version = new_table_message.UpdateEventTable.Version

	// Now delete the artifact completely
	repository_manager.DeleteArtifactFile(
		ctx, self.ConfigObj, "", "TestArtifact")

	// The table should magically be updated!
	table_json := ""
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		if !manager.CheckClientEventsVersion(
			context.Background(), self.ConfigObj, self.client_id, table_version) {
			return false
		}

		table := manager.GetClientUpdateEventTableMessage(
			context.Background(), self.ConfigObj, self.client_id)
		table_json = json.StringIndent(table)

		// The table should not contain the Crib any more
		return !strings.Contains(table_json, "TestArtifact")
	})

	// still contains the other artifacts
	assert.Contains(self.T(), table_json, "SomethingElse")
}

func (self *ClientMonitoringTestSuite) TestUpdatingClientTable() {
	closer := utils.MockTime(&utils.IncClock{NowTime: 10})
	defer closer()

	repository_manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository_manager.SetArtifactFile(self.Ctx, self.ConfigObj, "", `
name: TestArtifact
sources:
- query:
    SELECT * FROM info()
`, "")

	manager, err := services.ClientEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Set the initial table.
	err = manager.SetClientMonitoringState(
		context.Background(), self.ConfigObj, "",
		&flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"TestArtifact"},
			},
		})
	assert.NoError(self.T(), err)

	// Get the client's event table
	old_table := manager.GetClientUpdateEventTableMessage(
		context.Background(), self.ConfigObj, self.client_id)

	// Now update the monitoring state
	err = manager.SetClientMonitoringState(
		context.Background(), self.ConfigObj, "",
		&flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"TestArtifact"},
			},
		})
	assert.NoError(self.T(), err)

	// Get the client event table again
	table := manager.GetClientUpdateEventTableMessage(
		context.Background(), self.ConfigObj, self.client_id)

	// Make sure the client event table version is updated.
	assert.True(self.T(),
		table.UpdateEventTable.Version > old_table.UpdateEventTable.Version)
}

func (self *ClientMonitoringTestSuite) TestUpdatingClientTableMultiFrontend() {
	closer := utils.MockTime(&utils.IncClock{NowTime: 10})
	defer closer()

	repository_manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository_manager.SetArtifactFile(self.Ctx, self.ConfigObj, "", `
name: TestArtifact
sources:
- query:
    SELECT * FROM info()
`, "")

	manager1, err := services.ClientEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Set the initial table.
	err = manager1.SetClientMonitoringState(
		context.Background(), self.ConfigObj, "",
		&flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"TestArtifact"},
			},
		})
	assert.NoError(self.T(), err)

	// Get the client's event table
	old_table := manager1.GetClientUpdateEventTableMessage(
		context.Background(), self.ConfigObj, self.client_id)

	// Now another frontend sets the client monitoring state
	manager2, err := client_monitoring.NewClientMonitoringService(
		self.Ctx, self.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	// Now update the monitoring state
	err = manager2.SetClientMonitoringState(
		context.Background(), self.ConfigObj, "",
		&flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"TestArtifact"},
			},
		})
	assert.NoError(self.T(), err)

	// Get the client event table again
	table := manager2.GetClientUpdateEventTableMessage(
		context.Background(), self.ConfigObj, self.client_id)

	// Make sure the client event table version is updated.
	assert.True(self.T(),
		table.UpdateEventTable.Version > old_table.UpdateEventTable.Version)
}

func (self *ClientMonitoringTestSuite) TestClientMonitoringCompiling() {
	// Every time the clock gives time.Now() it is forced to
	// increment.
	closer := utils.MockTime(&utils.IncClock{NowTime: 10})
	defer closer()

	// If no table exists, we will get a default table.
	manager, err := services.ClientEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	labeler := services.GetLabeler(self.ConfigObj)

	// Install an initial monitoring table: Everyone gets ServiceCreation.
	manager.SetClientMonitoringState(
		context.Background(), self.ConfigObj, "",
		&flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"Windows.Events.ServiceCreation"},
			},
		})

	table := manager.GetClientUpdateEventTableMessage(
		context.Background(), self.ConfigObj, self.client_id)

	// There should be one event table sent.
	assert.Equal(self.T(), len(table.UpdateEventTable.Event), 1)

	version := table.UpdateEventTable.Version
	assert.True(self.T(), version > 0)

	// Now the client upgraded its table, do we need to update it again?
	assert.False(self.T(), manager.CheckClientEventsVersion(
		context.Background(), self.ConfigObj,
		self.client_id, version))

	// Add a label to the client
	require.NoError(self.T(), labeler.SetClientLabel(
		context.Background(), self.ConfigObj,
		self.client_id, "Label1"))

	// Since the client's label changed it might need to be updated.
	assert.True(self.T(), manager.CheckClientEventsVersion(
		context.Background(), self.ConfigObj,
		self.client_id, version))

	// But the event table does not include a rule for this label anyway.
	table = manager.GetClientUpdateEventTableMessage(
		context.Background(), self.ConfigObj, self.client_id)
	assert.Equal(self.T(), len(table.UpdateEventTable.Event), 1)

	// New table is still updated though.
	assert.True(self.T(), version < table.UpdateEventTable.Version)
	version = table.UpdateEventTable.Version

	// Now lets install a new label rule for this label and another label.
	manager.SetClientMonitoringState(
		context.Background(),
		self.ConfigObj, "", &flows_proto.ClientEventTable{
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
	assert.True(self.T(), manager.CheckClientEventsVersion(
		context.Background(), self.ConfigObj,
		self.client_id, version))

	// The new table includes 2 rules - the default and for Label1
	table = manager.GetClientUpdateEventTableMessage(
		context.Background(), self.ConfigObj, self.client_id)
	assert.Equal(self.T(), len(table.UpdateEventTable.Event), 2)

	// New table has a later version.
	assert.True(self.T(), version < table.UpdateEventTable.Version)
	version = table.UpdateEventTable.Version

	// The client only has Label1 set so should only receive ServiceCreation and DNSQueries
	assert.Equal(self.T(), extractArtifacts(table.UpdateEventTable),
		[]string{"Windows.Events.ServiceCreation", "Windows.Events.DNSQueries"})

	// Lets add Label2 to this client.
	labeler.SetClientLabel(context.Background(), self.ConfigObj, self.client_id, "Label2")

	// A new table is installed, this client must update.
	assert.True(self.T(), manager.CheckClientEventsVersion(
		context.Background(), self.ConfigObj,
		self.client_id, version))

	// The new table includes 3 rules - the default and for Label1 and Label2
	table = manager.GetClientUpdateEventTableMessage(
		context.Background(), self.ConfigObj, self.client_id)
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
	assert.False(self.T(), manager.CheckClientEventsVersion(
		context.Background(), self.ConfigObj,
		self.client_id, version))
}

// Event queries are asyncronous and blocking so when collecting
// multiple queries, we need to send each query in its own Event entry
// so they can run in parallel. The client runs each Event object in a
// separate goroutine. It is not allowed to send multiple SELECT
// statements in the same event because this will block on the first
// SELECT and never reach the second SELECT. This test checks for this
// condition.
func (self *ClientMonitoringTestSuite) TestClientMonitoringCompilingMultipleArtifacts() {
	closer := utils.MockTime(&utils.IncClock{NowTime: 10})
	defer closer()

	// If no table exists, we will get a default table.
	manager, err := services.ClientEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Install an initial monitoring table: Everyone gets ServiceCreation.
	manager.SetClientMonitoringState(
		context.Background(),
		self.ConfigObj, "", &flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{
					"Windows.Events.ServiceCreation",
					"Windows.Events.ProcessCreation",
				},
			},
		})
	table := manager.GetClientUpdateEventTableMessage(
		context.Background(), self.ConfigObj, self.client_id)

	// Count how many SELECT statements exist in each event table.
	for _, event := range table.UpdateEventTable.Event {
		count := 0
		// Make sure we have a dedicated precondition in each event.
		assert.Contains(self.T(), event.Precondition, "SELECT")
		for _, query := range event.Query {
			if strings.HasPrefix(query.VQL, "SELECT") {
				count++
				// Make sure it contains the precondition
				assert.Contains(self.T(), query.VQL, "precondition_")
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
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 0)))
	defer closer()

	// If no table exists, we will get a default table.
	manager, err := services.ClientEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	test_utils.GetMemoryDataStore(self.T(), self.ConfigObj).Clear()
	assert.NoError(self.T(),
		manager.(*client_monitoring.ClientEventTable).LoadFromFile(
			context.Background(), self.ConfigObj))

	table := manager.GetClientMonitoringState()

	// Version is based on timestamp.
	assert.Equal(self.T(), table.Version, uint64(10000000000))
	assert.Equal(self.T(), table.Artifacts.Artifacts, []string{"Generic.Client.Stats"})

	// If a client presents an earlier version table they will
	// need to update.
	assert.True(self.T(), manager.CheckClientEventsVersion(
		context.Background(), self.ConfigObj,
		self.client_id, 50))

	// If a client presents the same table version they dont need to do anything.
	assert.False(self.T(), manager.CheckClientEventsVersion(
		context.Background(), self.ConfigObj,
		self.client_id, uint64(10000000000)))

	// Some time later we label the client.
	closer = utils.MockTime(utils.NewMockClock(time.Unix(20, 0)))
	defer closer()

	labeler := services.GetLabeler(self.ConfigObj)
	labeler.SetClientLabel(self.Ctx, self.ConfigObj, self.client_id, "Foobar")

	// Client will now be required to update its event table to
	// make sure the new label does not apply.
	assert.True(self.T(), manager.CheckClientEventsVersion(
		self.Ctx, self.ConfigObj,
		self.client_id, uint64(10000000000)))

}

func TestClientMonitoringService(t *testing.T) {
	suite.Run(t, &ClientMonitoringTestSuite{})
}
