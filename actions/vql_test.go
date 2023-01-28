package actions_test

import (
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type ClientVQLTestSuite struct {
	test_utils.TestSuite
}

func (self *ClientVQLTestSuite) TestCPUThrottler() {
	request := &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			{
				Name: "Query",
				VQL:  "SELECT 'Boo' FROM scope()",
			},
		},
	}

	// Query is not limited
	resp := responder.TestResponderWithFlowId(
		self.ConfigObj, "TestCPUThrottler")
	actions.VQLClientAction{}.StartQuery(
		self.ConfigObj, self.Sm.Ctx, resp, request)
	resp.Close()

	assert.NotContains(self.T(), getLogs(resp.Drain.Messages()),
		"Will throttle query")

	// Query will now be limited
	resp = responder.TestResponderWithFlowId(
		self.ConfigObj, "TestCPUThrottler2")
	defer resp.Close()

	request.CpuLimit = 20
	actions.VQLClientAction{}.StartQuery(
		self.ConfigObj, self.Sm.Ctx, resp, request)

	var responses []*crypto_proto.VeloMessage
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		responses = resp.Drain.Messages()
		return strings.Contains(getLogs(responses), "Will throttle query")
	})

	assert.Contains(self.T(), getLogs(responses), "Will throttle query")
}

// Make sure that dependent artifacts are properly used
func (self *ClientVQLTestSuite) TestDependentArtifacts() {
	resp := responder.TestResponderWithFlowId(
		self.ConfigObj, "TestDependentArtifacts")

	actions.VQLClientAction{}.StartQuery(self.ConfigObj, self.Sm.Ctx, resp,
		&actions_proto.VQLCollectorArgs{
			Query: []*actions_proto.VQLRequest{
				{
					Name: "Query",
					VQL:  "SELECT * FROM Artifact.Custom.Foo.Bar.Baz.A()",
				},
			},
			Artifacts: []*artifacts_proto.Artifact{
				{
					Name: "Custom.Foo.Bar.Baz.A",
					Sources: []*artifacts_proto.ArtifactSource{
						{
							Query: "SELECT * FROM Artifact.Custom.Foo.Bar.Baz.B()",
						},
					},
				},
				{
					Name: "Custom.Foo.Bar.Baz.B",
					Sources: []*artifacts_proto.ArtifactSource{
						{
							Query: "SELECT * FROM Artifact.Custom.Foo.Bar.Baz.C()",
						},
					},
				},
				{
					Name: "Custom.Foo.Bar.Baz.C",
					Sources: []*artifacts_proto.ArtifactSource{
						{
							Query: "SELECT 1 AS X FROM scope()",
						},
					},
				},
			},
		})

	assert.Equal(self.T(), "{\"X\":1,\"_Source\":\"Custom.Foo.Bar.Baz.A\"}\n",
		getVQLResponse(resp.Drain.Messages()))
}

func getLogs(responses []*crypto_proto.VeloMessage) string {
	result := ""
	for _, item := range responses {
		if item.LogMessage != nil {
			result += item.LogMessage.Jsonl + "\n"
		}
	}

	return result
}

func getVQLResponse(responses []*crypto_proto.VeloMessage) string {
	for _, item := range responses {
		if item.VQLResponse != nil {
			return item.VQLResponse.JSONLResponse
		}
	}

	return ""
}

func TestClientVQL(t *testing.T) {
	suite.Run(t, &ClientVQLTestSuite{})
}
