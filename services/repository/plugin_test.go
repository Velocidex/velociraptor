/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
package repository_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/actions"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"

	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/common"
)

// The test artifact plugin: This is the gateway to calling other
// artifacts from within VQL.
type PluginTestSuite struct {
	test_utils.TestSuite
}

// Load all built in artifacts and make sure they validate
// syntax. This should catch syntax errors in built in artifacts.
func (self *PluginTestSuite) TestArtifactsSyntax() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	logging.ClearMemoryLogs()

	err = repository.LoadBuiltInArtifacts(
		self.Ctx, self.ConfigObj, manager.(*repository.RepositoryManager))
	assert.NoError(self.T(), err)

	assert.NotContains(self.T(),
		strings.Join(logging.GetMemoryLogs(), "\n"),
		"Cant parse asset")

	ConfigObj := self.ConfigObj
	repository, err := manager.GetGlobalRepository(ConfigObj)
	assert.NoError(self.T(), err)

	new_repository := manager.NewRepository()
	new_repository.SetParent(repository, ConfigObj)

	names, err := repository.List(self.Ctx, ConfigObj)
	assert.NoError(self.T(), err)

	for _, artifact_name := range names {
		state := launcher.NewAnalysisState(artifact_name)
		artifact, pres := repository.Get(self.Ctx, ConfigObj, artifact_name)
		assert.True(self.T(), pres)

		if artifact != nil && !artifact.IsAlias {
			_, err = new_repository.LoadProto(artifact,
				services.ArtifactOptions{ValidateArtifact: true})
			assert.NoError(self.T(), err, "Error compiling "+artifact_name)

			launcher.VerifyArtifact(
				self.Ctx, self.ConfigObj, new_repository, artifact, state)

			for _, err := range state.Errors {
				fmt.Printf("Error with %v: %v\n", artifact_name, err)
			}
			assert.True(self.T(), len(state.Errors) == 0)
		}
	}
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
	repository := self.LoadArtifacts(artifact_definitions_precondition...)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Repository: repository,
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	scope := manager.BuildScope(builder)
	defer scope.Close()

	queries := []string{
		"SELECT * FROM Artifact.CallArtifactWithFalsePrecondition()",

		// Preconditions are **not** enforced on the sub artifact
		// because we only ask the top artifact to honor them, but it
		// is **not** also passing the preconditions requirement to
		// its sub artifact.
		"SELECT * FROM Artifact.CallArtifactWithFalsePrecondition(preconditions=TRUE)",
	}

	results := ordereddict.NewDict()
	for _, query := range queries {
		rows := []vfilter.Row{}
		vql, err := vfilter.Parse(query)
		assert.NoError(self.T(), err)

		for row := range vql.Eval(self.Ctx, scope) {
			rows = append(rows, row)
		}
		results.Set(query, rows)
	}

	goldie.AssertJson(self.T(), "TestArtifactPluginWithPrecondition", results)
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
	repository := self.LoadArtifacts(source_definitions...)
	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  "C.1234",
		Artifacts: []string{"Call"},
	}

	acl_manager := acl_managers.NullACLManager{}
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	compiled, err := launcher.CompileCollectorArgs(
		self.Ctx, self.ConfigObj, acl_manager, repository,
		services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	test_responder := responder.TestResponderWithFlowId(
		self.ConfigObj, "F.TestClientPluginMultipleSources")
	for _, vql_request := range compiled {
		actions.VQLClientAction{}.StartQuery(
			self.ConfigObj, self.Ctx, test_responder, vql_request)
	}
	defer test_responder.Close()

	messages := []*crypto_proto.VeloMessage{}
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		messages = test_responder.Drain.Messages()
		return len(messages) >= 2
	})

	results := ""
	for _, msg := range messages {
		if msg.VQLResponse != nil {
			results += msg.VQLResponse.JSONLResponse
		}
	}

	goldie.Assert(self.T(), "TestClientPluginMultipleSources", []byte(results))
}

var (
	precondition_source_definitions = []string{`
name: ArtifactWithSourcesAndPreconditions
type: CLIENT
sources:
- name: Source1
  precondition: SELECT * FROM info() WHERE FALSE
  query: SELECT "A" AS Column FROM scope()

- name: Source2
  precondition: SELECT * FROM info()
  query: SELECT "B" AS Column FROM scope()`}
)

// Test that calling a client artifact with multiple sources results
// in all rows.
func (self *PluginTestSuite) TestClientPluginMultipleSourcesAndPrecondtions() {
	repository := self.LoadArtifacts(precondition_source_definitions...)
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Repository: repository,
		Logger: logging.NewPlainLogger(
			self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	scope := manager.BuildScope(builder)
	defer scope.Close()

	queries := []string{
		"SELECT * FROM Artifact.ArtifactWithSourcesAndPreconditions()",
		"SELECT * FROM Artifact.ArtifactWithSourcesAndPreconditions(preconditions=TRUE)",

		// Select a specific source.
		"SELECT * FROM Artifact.ArtifactWithSourcesAndPreconditions(source='Source1')",

		// Should return no results as preconditions is false.
		"SELECT * FROM Artifact.ArtifactWithSourcesAndPreconditions(source='Source1', preconditions=TRUE)",
	}

	results := ordereddict.NewDict()
	for _, query := range queries {
		rows := []vfilter.Row{}
		vql, err := vfilter.Parse(query)
		assert.NoError(self.T(), err)

		for row := range vql.Eval(self.Ctx, scope) {
			rows = append(rows, row)
		}
		results.Set(query, rows)
	}

	goldie.AssertJson(
		self.T(), "TestClientPluginMultipleSourcesAndPrecondtions", results)

}

var (
	precondition_source_events_definitions = []string{`
name: ArtifactWithSourcesAndPreconditionsEvent
type: CLIENT_EVENT
sources:
- name: Source1
  precondition: SELECT * FROM info() WHERE FALSE
  query: SELECT "A" AS Column FROM clock(ms=100) LIMIT 2

- name: Source2
  precondition: SELECT * FROM info()
  query: SELECT "B" AS Column FROM clock(ms=100) LIMIT 2
`}
)

// Test that calling a client artifact with multiple sources results
// in all rows.
func (self *PluginTestSuite) TestClientPluginMultipleSourcesAndPrecondtionsEvents() {
	repository := self.LoadArtifacts(precondition_source_events_definitions...)
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Repository: repository,
		Logger: logging.NewPlainLogger(
			self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	scope := manager.BuildScope(builder)
	defer scope.Close()

	queries := []string{
		"SELECT * FROM Artifact.ArtifactWithSourcesAndPreconditionsEvent() ORDER BY Column",
		"SELECT * FROM Artifact.ArtifactWithSourcesAndPreconditionsEvent(preconditions=TRUE) ORDER BY Column",
	}

	results := ordereddict.NewDict()
	for _, query := range queries {
		rows := []vfilter.Row{}
		vql, err := vfilter.Parse(query)
		assert.NoError(self.T(), err)

		for row := range vql.Eval(self.Ctx, scope) {
			rows = append(rows, row)
		}
		results.Set(query, rows)
	}

	goldie.AssertJson(self.T(),
		"TestClientPluginMultipleSourcesAndPrecondtionsEvents", results)

}

func TestArtifactPlugin(t *testing.T) {
	suite.Run(t, &PluginTestSuite{})
}
