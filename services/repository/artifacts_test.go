/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/vql/common"
)

// Load all built in artifacts and make sure they validate
// syntax. This should catch syntax errors in built in artifacts.
func TestArtifactsSyntax(t *testing.T) {
	config_obj, err := new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(t, err)

	sm := services.NewServiceManager(context.Background(), config_obj)
	defer sm.Close()

	assert.NoError(t, sm.Start(journal.StartJournalService))
	assert.NoError(t, sm.Start(notifications.StartNotificationService))
	assert.NoError(t, sm.Start(inventory.StartInventoryService))
	assert.NoError(t, sm.Start(StartRepositoryManager))

	manager, err := services.GetRepositoryManager()
	assert.NoError(t, err)

	repository, err := manager.GetGlobalRepository(config_obj)
	assert.NoError(t, err)

	new_repository := manager.NewRepository()

	for _, artifact_name := range repository.List() {
		artifact, pres := repository.Get(config_obj, artifact_name)
		assert.True(t, pres)

		if artifact != nil {
			_, err = new_repository.LoadProto(artifact, true /* validate */)
			assert.NoError(t, err, "Error compiling "+artifact_name)
		}
	}
}

// The test artifact plugin: This is the gateway to calling other
// artifacts from within VQL.
type PluginTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	sm         *services.Service
	ctx        context.Context
	cancel     func()
}

func (self *PluginTestSuite) SetupTest() {
	self.config_obj = config.GetDefaultConfig()
	self.config_obj.Datastore.Implementation = "Test"

	// Start essential services.
	self.ctx, self.cancel = context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(self.ctx, self.config_obj)

	assert.NoError(self.T(), self.sm.Start(StartRepositoryManagerForTest))
	assert.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	assert.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	assert.NoError(self.T(), self.sm.Start(inventory.StartInventoryService))
	assert.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))
}

func (self *PluginTestSuite) TearDownTest() {
	self.sm.Close()
	self.cancel()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

func (self *PluginTestSuite) LoadArtifacts(artifact_definitions []string) services.Repository {
	manager, _ := services.GetRepositoryManager()
	repository := manager.NewRepository()

	for _, definition := range artifact_definitions {
		_, err := repository.LoadYaml(definition, false)
		assert.NoError(self.T(), err)
	}

	return repository
}

var (
	artifact_definitions = []string{`
name: Test1
sources:
- query: SELECT * FROM Artifact.Test1.Foobar()
`, `
name: Test1.Foobar
sources:
- query: SELECT * FROM info()
`, `
name: Category.Test1
sources:
- query: SELECT * FROM Artifact.Test1.Foobar()
`, `
name: Category.Test2
sources:
- query: SELECT * FROM info()
`}
)

func (self *PluginTestSuite) TestArtifactPlugin() {
	repository := self.LoadArtifacts(artifact_definitions)

	wg := &sync.WaitGroup{}
	p := NewArtifactRepositoryPlugin(wg, repository.(*Repository)).(*ArtifactRepositoryPlugin)

	g := goldie.New(self.T())
	g.Assert(self.T(), "TestArtifactPlugin", []byte(p.Print()))
}

var (
	artifact_definitions_precondition = []string{`
name: CallArtifactWithFalsePrecondition
sources:
- query: |
    SELECT * FROM Artifact.FalsePrecondition()
`, `
name: FalsePrecondition
sources:
- precondition: |
      SELECT 1 FROM scope() WHERE FALSE

  query: |
      SELECT 1 AS A FROM scope()
`}
)

func (self *PluginTestSuite) TestArtifactPluginWithPrecondition() {
	repository := self.LoadArtifacts(artifact_definitions_precondition)

	builder := services.ScopeBuilder{
		Config:     self.config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Repository: repository,
		Logger:     logging.NewPlainLogger(self.config_obj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	manager, _ := services.GetRepositoryManager()
	scope := manager.BuildScope(builder)
	defer scope.Close()

	queries := []string{
		"SELECT * FROM Artifact.CallArtifactWithFalsePrecondition()",
		"SELECT * FROM Artifact.CallArtifactWithFalsePrecondition(precondition=TRUE)",
	}

	results := ordereddict.NewDict()
	for _, query := range queries {
		rows := []vfilter.Row{}
		vql, err := vfilter.Parse(query)
		assert.NoError(self.T(), err)

		for row := range vql.Eval(self.ctx, scope) {
			rows = append(rows, row)
		}
		results.Set(query, rows)
	}

	g := goldie.New(self.T())
	g.Assert(self.T(), "TestArtifactPluginWithPrecondition", json.MustMarshalIndent(results))
}

var (
	event_definitions = []string{`
name: EventWithSources
type: CLIENT_EVENT
sources:
- name: Source1
  query: SELECT Unix FROM clock() LIMIT 1

- name: Source2
  query: SELECT Unix FROM clock() LIMIT 1`,
		`
name: Call
sources:
- query: SELECT * FROM Artifact.EventWithSources()
`}
)

// Test that calling an event artifact with multiple sources results
// in an error.
func (self *PluginTestSuite) TestEventPluginMultipleSources() {
	repository := self.LoadArtifacts(event_definitions)
	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  "C.1234",
		Artifacts: []string{"Call"},
	}

	acl_manager := vql_subsystem.NullACLManager{}
	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		self.ctx, self.config_obj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	test_responder := responder.TestResponder()
	for _, vql_request := range compiled {
		actions.VQLClientAction{}.StartQuery(
			self.config_obj, self.ctx,
			test_responder, vql_request)
	}

	logs := ""
	for _, msg := range responder.GetTestResponses(test_responder) {
		if msg.LogMessage != nil {
			logs += msg.LogMessage.Message
		}
	}

	assert.Contains(self.T(), logs,
		"Artifact EventWithSources is an artifact with multiple sources, please specify a source")
}

var (
	source_definitions = []string{`
name: ClientWithSources
type: CLIENT
sources:
- name: Source1
  query: SELECT "A" AS Column FROM scope()

- name: Source2
  query: SELECT "B" AS Column FROM scope()`,
		`
name: Call
sources:
- query: SELECT * FROM Artifact.ClientWithSources()
`}
)

// Test that calling a client artifact with multiple sources results
// in all rows.
func (self *PluginTestSuite) TestClientPluginMultipleSources() {
	repository := self.LoadArtifacts(source_definitions)
	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  "C.1234",
		Artifacts: []string{"Call"},
	}

	acl_manager := vql_subsystem.NullACLManager{}
	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		self.ctx, self.config_obj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	test_responder := responder.TestResponder()
	for _, vql_request := range compiled {
		actions.VQLClientAction{}.StartQuery(
			self.config_obj, self.ctx, test_responder, vql_request)
	}

	results := ""
	for _, msg := range responder.GetTestResponses(test_responder) {
		if msg.VQLResponse != nil {
			results += msg.VQLResponse.JSONLResponse
		}
	}
	g := goldie.New(self.T())
	g.Assert(self.T(), "TestClientPluginMultipleSources", []byte(results))
}

func TestArtifactPlugin(t *testing.T) {
	suite.Run(t, &PluginTestSuite{})
}
