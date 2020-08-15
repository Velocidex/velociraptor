package launcher

import (
	"context"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
	_ "www.velocidex.com/golang/velociraptor/vql_plugins"
)

var (
	test_artifact_definitions = []string{
		// Artifacts 1 and 2 are invalid: cyclic
		`
name: Artifact1
parameters:
- name: arg1
sources:
  - queries:
      - SELECT * FROM Artifact.Artifact2(arg1=log(message="Calling Artifact2"))
`, `
name: Artifact2
parameters:
- name: arg1
sources:
  - queries:
      - SELECT * FROM Artifact.Artifact1(arg1=log(message="Calling Artifact1"))
`,
		// Artifact3 has no dependency
		`
name: Artifact3
sources:
  - queries:
      - SELECT "Foobar" AS A FROM scope()
`,

		// Artifact4 depends on 3
		`
name: Artifact4
sources:
  - queries:
      - SELECT * FROM Artifact.Artifact3()
`,
		// Artifact6 depends on both 3 and 4, but there is no
		// cycle so should work fine.
		`
name: Artifact6
sources:
  - queries:
      - SELECT * FROM chain(
          a={ SELECT * FROM Artifact.Artifact3() },
          b={ SELECT * FROM Artifact.Artifact4() })
`,

		// Artifact5 depends on an unknown artifact
		`
name: Artifact5
sources:
  - queries:
      - SELECT * FROM Artifact.Unknown()
`}
)

type ArtifactTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	repository *artifacts.Repository
	sm         *services.Service
}

func (self *ArtifactTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	t := self.T()
	assert.NoError(t, self.sm.Start(journal.StartJournalService))
	assert.NoError(t, self.sm.Start(services.StartNotificationService))
	assert.NoError(t, self.sm.Start(inventory.StartInventoryService))
	assert.NoError(t, self.sm.Start(StartLauncherService))

	self.repository = artifacts.NewRepository()
	for _, definition := range test_artifact_definitions {
		self.repository.LoadYaml(definition, false)
	}
}

func (self *ArtifactTestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

func (self *ArtifactTestSuite) TestUnknownArtifact() {
	// Broken - depends on an unknown artifact
	request := &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				VQL: "SELECT * FROM Artifact.Artifact5()",
			},
		},
	}
	err := self.repository.PopulateArtifactsVQLCollectorArgs(request)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "Unknown artifact reference")
}

// Check that execution is aborted when recursion occurs.
func (self *ArtifactTestSuite) TestStackOverflow() {
	// Cycle: Artifact1 -> Artifact2 -> Artifact1
	request := &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				VQL: "SELECT * FROM Artifact.Artifact1()",
			},
		},
	}

	// It should compile ok but overflow at runtime.
	err := self.repository.PopulateArtifactsVQLCollectorArgs(request)
	assert.NoError(self.T(), err)

	// If we fail this test make sure we take a resonable time.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	test_responder := responder.TestResponder()

	actions.VQLClientAction{}.StartQuery(
		self.config_obj, ctx, test_responder, request)

	assert.Contains(self.T(), getLogMessages(test_responder),
		"Stack overflow: Artifact1, Artifact2, Artifact1, Artifact2")
}

func (self *ArtifactTestSuite) TestArtifactDependencies() {
	// Artifact6 -> Artifact3
	//           -> Artifact4 -> Artifact3
	request := &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				VQL: "SELECT * FROM Artifact.Artifact6()",
			},
		},
	}
	err := self.repository.PopulateArtifactsVQLCollectorArgs(request)
	assert.NoError(self.T(), err)

	// If we fail make sure we take a resonable time.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	test_responder := responder.TestResponder()

	actions.VQLClientAction{}.StartQuery(
		self.config_obj, ctx, test_responder, request)

	results := getResponses(test_responder)

	// Return both rows, one from Artifact4 and one from Artifact3
	assert.Equal(self.T(), len(results), 2)
	a, _ := results[0].Get("A")
	assert.Equal(self.T(), a, "Foobar")
	a, _ = results[1].Get("A")
	assert.Equal(self.T(), a, "Foobar")
}

func getLogMessages(r *responder.Responder) string {
	result := ""
	for _, msg := range responder.GetTestResponses(r) {
		if msg.LogMessage != nil {
			result += msg.LogMessage.Message
		}
	}

	return result
}

func getResponses(r *responder.Responder) []*ordereddict.Dict {
	result := []*ordereddict.Dict{}

	for _, msg := range responder.GetTestResponses(r) {
		if msg.VQLResponse != nil {
			payload, err := utils.ParseJsonToDicts(
				[]byte(msg.VQLResponse.Response))
			if err != nil {
				continue
			}

			result = append(result, payload...)
		}
	}
	return result
}

func TestArtifactCompiling(t *testing.T) {
	suite.Run(t, &ArtifactTestSuite{})
}
