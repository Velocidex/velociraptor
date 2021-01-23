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

	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/config"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"

	_ "www.velocidex.com/golang/velociraptor/vql/common"
)

// Load all built in artifacts and make sure they validate syntax.
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

func TestArtifactPlugin(t *testing.T) {
	config_obj := config.GetDefaultConfig()

	sm := services.NewServiceManager(context.Background(), config_obj)
	defer sm.Close()

	assert.NoError(t, sm.Start(journal.StartJournalService))
	assert.NoError(t, sm.Start(notifications.StartNotificationService))
	assert.NoError(t, sm.Start(inventory.StartInventoryService))
	assert.NoError(t, sm.Start(StartRepositoryManager))

	manager, _ := services.GetRepositoryManager()
	repository := manager.NewRepository()

	for _, definition := range artifact_definitions {
		_, err := repository.LoadYaml(definition, false)
		assert.NoError(t, err)
	}

	wg := &sync.WaitGroup{}
	p := NewArtifactRepositoryPlugin(wg, repository.(*Repository)).(*ArtifactRepositoryPlugin)

	g := goldie.New(t)
	g.Assert(t, "TestArtifactPlugin", []byte(p.Print()))
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
func TestEventPluginMultipleSources(t *testing.T) {
	config_obj := config.GetDefaultConfig()

	sm := services.NewServiceManager(context.Background(), config_obj)
	defer sm.Close()

	assert.NoError(t, sm.Start(journal.StartJournalService))
	assert.NoError(t, sm.Start(notifications.StartNotificationService))
	assert.NoError(t, sm.Start(inventory.StartInventoryService))
	assert.NoError(t, sm.Start(launcher.StartLauncherService))
	assert.NoError(t, sm.Start(StartRepositoryManager))

	manager, _ := services.GetRepositoryManager()
	repository, _ := manager.GetGlobalRepository(config_obj)

	for _, definition := range event_definitions {
		_, err := repository.LoadYaml(definition, true)
		assert.NoError(t, err)
	}

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  "C.1234",
		Artifacts: []string{"Call"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	acl_manager := vql_subsystem.NullACLManager{}
	launcher, err := services.GetLauncher()
	assert.NoError(t, err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, config_obj, acl_manager, repository, false, request)
	assert.NoError(t, err)

	test_responder := responder.TestResponder()
	for _, vql_request := range compiled {
		actions.VQLClientAction{}.StartQuery(
			config_obj, ctx, test_responder, vql_request)
	}

	logs := ""
	for _, msg := range responder.GetTestResponses(test_responder) {
		if msg.LogMessage != nil {
			logs += msg.LogMessage.Message
		}
	}

	assert.Contains(t, logs, "Artifact EventWithSources is an event artifact with multiple sources, please specify a source")
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
func TestClientPluginMultipleSources(t *testing.T) {
	config_obj := config.GetDefaultConfig()

	sm := services.NewServiceManager(context.Background(), config_obj)
	defer sm.Close()

	assert.NoError(t, sm.Start(journal.StartJournalService))
	assert.NoError(t, sm.Start(notifications.StartNotificationService))
	assert.NoError(t, sm.Start(inventory.StartInventoryService))
	assert.NoError(t, sm.Start(launcher.StartLauncherService))
	assert.NoError(t, sm.Start(StartRepositoryManager))

	manager, _ := services.GetRepositoryManager()
	repository, _ := manager.GetGlobalRepository(config_obj)

	for _, definition := range source_definitions {
		_, err := repository.LoadYaml(definition, true)
		assert.NoError(t, err)
	}

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  "C.1234",
		Artifacts: []string{"Call"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	acl_manager := vql_subsystem.NullACLManager{}
	launcher, err := services.GetLauncher()
	assert.NoError(t, err)

	compiled, err := launcher.CompileCollectorArgs(
		ctx, config_obj, acl_manager, repository, false, request)
	assert.NoError(t, err)

	test_responder := responder.TestResponder()
	for _, vql_request := range compiled {
		actions.VQLClientAction{}.StartQuery(
			config_obj, ctx, test_responder, vql_request)
	}

	results := ""
	for _, msg := range responder.GetTestResponses(test_responder) {
		if msg.VQLResponse != nil {
			results += msg.VQLResponse.JSONLResponse
		}
	}
	g := goldie.New(t)
	g.Assert(t, "TestClientPluginMultipleSources", []byte(results))
}
