package actions_test

import (
	"testing"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/responder"
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
	resp := responder.TestResponder()
	actions.VQLClientAction{}.StartQuery(self.ConfigObj, self.Sm.Ctx, resp, request)
	assert.NotContains(self.T(), getLogs(resp), "Will throttle query")

	// Query will now be limited
	resp = responder.TestResponder()
	request.CpuLimit = 20
	actions.VQLClientAction{}.StartQuery(self.ConfigObj, self.Sm.Ctx, resp, request)
	assert.Contains(self.T(), getLogs(resp), "Will throttle query")
}

// Make sure that dependent artifacts are properly used
func (self *ClientVQLTestSuite) TestDependentArtifacts() {
	resp := responder.TestResponder()

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

	assert.Equal(self.T(), "{\"X\":1,\"_Source\":\"Custom.Foo.Bar.Baz.A\"}\n", getVQLResponse(resp))
}

func getLogs(resp *responder.Responder) string {
	result := ""
	responses := responder.GetTestResponses(resp)
	for _, item := range responses {
		if item.LogMessage != nil {
			result += item.LogMessage.Message + "\n"
		}
	}

	return result
}

func getVQLResponse(resp *responder.Responder) string {
	responses := responder.GetTestResponses(resp)
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
