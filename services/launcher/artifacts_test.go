package launcher_test

import (
	"context"
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
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
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
	test_utils.TestSuite
	repository services.Repository
}

func (self *ArtifactTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	self.repository = manager.NewRepository()
	for _, definition := range test_artifact_definitions {
		self.repository.LoadYaml(definition,
			services.ArtifactOptions{
				ValidateArtifact:  false,
				ArtifactIsBuiltIn: true})

	}
}

func (self *ArtifactTestSuite) TestUnknownArtifact() {
	// Broken - depends on an unknown artifact
	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{"Artifact5"},
	}

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	_, err = launcher.CompileCollectorArgs(context.Background(), self.ConfigObj,
		acl_managers.NullACLManager{},
		self.repository, services.CompilerOptions{}, request)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "Unknown artifact reference")
}

// Check that execution is aborted when recursion occurs.
func (self *ArtifactTestSuite) TestStackOverflow() {
	// Cycle: Artifact1 -> Artifact2 -> Artifact1
	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{"Artifact1"},
	}

	// It should compile ok but overflow at runtime.
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)
	vql_requests, err := launcher.CompileCollectorArgs(context.Background(),
		self.ConfigObj, acl_managers.NullACLManager{},
		self.repository, services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// If we fail this test make sure we take a resonable time.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	test_responder := responder.TestResponderWithFlowId(
		self.ConfigObj, "F.TestStackOverflow")
	for _, vql_request := range vql_requests {
		actions.VQLClientAction{}.StartQuery(
			self.ConfigObj, ctx, test_responder, vql_request)
	}
	defer test_responder.Close()

	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		messages := test_responder.Drain.Messages()
		return strings.Contains(getLogMessages(messages),
			"Stack overflow: Artifact2, Artifact1, Artifact2, Artifact1")
	})
}

func (self *ArtifactTestSuite) TestArtifactDependencies() {
	// Artifact6 -> Artifact3
	//           -> Artifact4 -> Artifact3
	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{"Artifact6"},
	}
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	vql_requests, err := launcher.CompileCollectorArgs(context.Background(),
		self.ConfigObj, acl_managers.NullACLManager{},
		self.repository, services.CompilerOptions{}, request)
	assert.NoError(self.T(), err)

	// If we fail make sure we take a resonable time.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	test_responder := responder.TestResponderWithFlowId(
		self.ConfigObj, "F.TestArtifactDependencies")

	for _, vql_request := range vql_requests {
		actions.VQLClientAction{}.StartQuery(
			self.ConfigObj, ctx, test_responder, vql_request)
	}

	var messages []*ordereddict.Dict
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		messages = getResponses(test_responder.Drain.Messages())
		return len(messages) == 2
	})

	// Return both rows, one from Artifact4 and one from Artifact3
	assert.Equal(self.T(), len(messages), 2)
	a, _ := messages[0].Get("A")
	assert.Equal(self.T(), a, "Foobar")
	a, _ = messages[1].Get("A")
	assert.Equal(self.T(), a, "Foobar")
}

func getLogMessages(messages []*crypto_proto.VeloMessage) string {
	result := ""
	for _, msg := range messages {
		if msg.LogMessage != nil {
			result += msg.LogMessage.Jsonl
		}
	}

	return result
}

func getResponses(messages []*crypto_proto.VeloMessage) []*ordereddict.Dict {
	result := []*ordereddict.Dict{}

	for _, msg := range messages {
		if msg.VQLResponse != nil {
			payload, err := utils.ParseJsonToDicts(
				[]byte(msg.VQLResponse.JSONLResponse))
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
