package monitoring

import (
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_monitoring"
	"www.velocidex.com/golang/velociraptor/services/server_monitoring"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

var (
	definitions = []string{
		`
name: Linux.Events.SSHLogin
type: CLIENT_EVENT
parameters:
 - name: syslogAuthLogPath
`, `
name: System.Hunt.Creation
type: SERVER_EVENT
sources:
- query: SELECT * FROM scope()
`, `
name: Server.Monitor.Health
type: SERVER_EVENT
sources:
- query: SELECT * FROM scope()
`,
	}
)

type MonitoringTestSuite struct {
	test_utils.TestSuite
}

func (self *MonitoringTestSuite) SetupTest() {
	self.TestSuite.SetupTest()
	self.LoadArtifacts(definitions)

	require.NoError(self.T(), self.Sm.Start(
		client_monitoring.StartClientMonitoringService))

	require.NoError(self.T(), self.Sm.Start(
		server_monitoring.StartServerMonitoringService))
}

func (self *MonitoringTestSuite) TestAddClientMonitoringNoPermissions() {
	log_buffer := &strings.Builder{}

	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(log_buffer, "", 0))

	sub_ctx, cancel := context.WithTimeout(self.Sm.Ctx, time.Second)
	defer cancel()

	res := AddClientMonitoringFunction{}.Call(
		sub_ctx, scope, ordereddict.NewDict().
			Set("artifact", "NoSuchArtifact"))
	assert.Equal(self.T(), res, vfilter.Null{})

	assert.Contains(self.T(), log_buffer.String(), "Permission denied:")
	log_buffer.Reset()
}

func (self *MonitoringTestSuite) TestAddServerMonitoringNoPermissions() {
	log_buffer := &strings.Builder{}

	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(log_buffer, "", 0))

	sub_ctx, cancel := context.WithTimeout(self.Sm.Ctx, time.Second)
	defer cancel()

	res := AddServerMonitoringFunction{}.Call(
		sub_ctx, scope, ordereddict.NewDict().
			Set("artifact", "NoSuchArtifact"))
	assert.Equal(self.T(), res, vfilter.Null{})

	assert.Contains(self.T(), log_buffer.String(), "Permission denied:")
	log_buffer.Reset()
}

func (self *MonitoringTestSuite) TestAddServerMonitoring() {
	log_buffer := &strings.Builder{}

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(log_buffer, "vql: ", 0),
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	sub_ctx, cancel := context.WithTimeout(self.Sm.Ctx, time.Second)
	defer cancel()

	res := AddServerMonitoringFunction{}.Call(
		sub_ctx, scope, ordereddict.NewDict().
			Set("artifact", "NoSuchArtifact"))
	assert.Equal(self.T(), res, vfilter.Null{})
	assert.Contains(self.T(), log_buffer.String(), "NoSuchArtifact not found")

	log_buffer.Reset()

	// Try to add a regular artifact
	res = AddServerMonitoringFunction{}.Call(
		sub_ctx, scope, ordereddict.NewDict().
			Set("artifact", "Generic.Client.Info"))
	assert.Equal(self.T(), res, vfilter.Null{})
	assert.Contains(self.T(), log_buffer.String(), "is not a server event artifact")

	log_buffer.Reset()

	_ = AddServerMonitoringFunction{}.Call(
		sub_ctx, scope, ordereddict.NewDict().
			Set("artifact", "System.Hunt.Creation").
			Set("parameters", ordereddict.NewDict().
				Set("syslogAuthLogPath", "AppliesToAll")))

	// Load the table from the service manager.
	monitoring_table := services.ServerEventManager.Get()

	golden := ordereddict.NewDict().
		Set("Add artifact", monitoring_table)

	// Now remove the artifact from the label.
	_ = RemoveServerMonitoringFunction{}.Call(
		sub_ctx, scope, ordereddict.NewDict().
			Set("artifact", "System.Hunt.Creation"))

	monitoring_table = services.ServerEventManager.Get()

	golden.Set("Removing artifact from label", monitoring_table)

	goldie.Assert(self.T(), "TestAddServerMonitoring",
		json.MustMarshalIndent(golden))
}

func TestMonitoringPlugins(t *testing.T) {
	suite.Run(t, &MonitoringTestSuite{})
}
