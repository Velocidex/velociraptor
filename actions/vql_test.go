package actions

import (
	"context"
	"testing"
	"time"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
)

type ClientVQLTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	sm         *services.Service
	ctx        context.Context
}

func (self *ClientVQLTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../http_comms/test_data/client.config.yaml").
		WithRequiredClient().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	// Start essential services.
	self.ctx, _ = context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(self.ctx, self.config_obj)

	t := self.T()
	assert.NoError(t, self.sm.Start(journal.StartJournalService))
	assert.NoError(t, self.sm.Start(notifications.StartNotificationService))
	assert.NoError(t, self.sm.Start(inventory.StartInventoryService))
	assert.NoError(t, self.sm.Start(launcher.StartLauncherService))
	assert.NoError(t, self.sm.Start(repository.StartRepositoryManager))
}

func (self *ClientVQLTestSuite) TearDownTest() {
	self.sm.Close()
}

// Make sure that dependent artifacts are properly used
func (self *ClientVQLTestSuite) TestDependentArtifacts() {
	resp := responder.TestResponder()

	VQLClientAction{}.StartQuery(self.config_obj, self.ctx, resp,
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
