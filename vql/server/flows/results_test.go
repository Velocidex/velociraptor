package flows

import (
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var (
	sourcePluginTestCases = []struct {
		Env  *ordereddict.Dict
		Args *ordereddict.Dict
		Mode pluginMode
	}{
		// Check implied parameters from environment.
		{
			// Client flow notebook provides the following in env.
			ordereddict.NewDict().
				Set("ArtifactName", "Generic.Client.Info").
				Set("ClientId", "C.123").
				Set("FlowId", "F.123"),
			ordereddict.NewDict(),
			MODE_FLOW_ARTIFACT,
		},
		{
			// Hunt notebooks provides the following in env.
			ordereddict.NewDict().
				Set("ArtifactName", "Generic.Client.Info").
				Set("HuntId", "H.123"),
			ordereddict.NewDict(),
			MODE_HUNT_ARTIFACT,
		},
		{
			// Client event notebook provides the following in env.
			ordereddict.NewDict().
				Set("ArtifactName", "Server.Monitor.Health").
				Set("ClientId", "C.123").
				Set("StartTime", "2025-07-14T23:59:53Z"),
			ordereddict.NewDict(),
			MODE_EVENT_ARTIFACT,
		},

		// Pass parameters in notebook
		{
			// Client event notebook provides the following in env.
			ordereddict.NewDict().
				Set("ArtifactName", "Server.Monitor.Health").
				Set("ClientId", "C.123").
				Set("StartTime", "2025-07-14T23:59:53Z"),
			ordereddict.NewDict().
				Set("notebook_id", "N.123").
				Set("notebook_cell_id", "NC.1234"),
			MODE_NOTEBOOK,
		},
	}
)

func (self *TestSuite) TestSourcePlugin() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	for _, tc := range sourcePluginTestCases {
		builder := services.ScopeBuilder{
			Config:     self.ConfigObj,
			ACLManager: acl_managers.NullACLManager{},
			Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),

			// All notebooks always set these.
			Env: tc.Env.
				Set("NotebookId", "N.1234").
				Set("NotebookCellId", "NC.1234"),
		}
		scope := manager.BuildScope(builder)
		defer scope.Close()

		arg := &SourcePluginArgs{}
		err = arg.DetermineMode(self.Ctx, self.ConfigObj, scope, tc.Args)
		assert.NoError(self.T(), err)

		assert.Equal(self.T(), tc.Mode, arg.mode)
	}
}
