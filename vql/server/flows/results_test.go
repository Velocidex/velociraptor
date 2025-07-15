package flows

import (
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	sourcePluginTestCases = []struct {
		Env       *ordereddict.Dict
		Args      *ordereddict.Dict
		Mode      pluginMode
		Assertion func(arg *SourcePluginArgs) bool
	}{
		// Check implied parameters from environment.
		{
			// Client flow notebook provides the following in env.
			Env: ordereddict.NewDict().
				Set("ArtifactName", "Generic.Client.Info").
				Set("ClientId", "C.123").
				Set("FlowId", "F.123"),
			Args: ordereddict.NewDict(),
			Mode: MODE_FLOW_ARTIFACT,
		},
		{
			// Hunt notebooks provides the following in env.
			Env: ordereddict.NewDict().
				Set("ArtifactName", "Generic.Client.Info").
				Set("HuntId", "H.123"),
			Args: ordereddict.NewDict(),
			Mode: MODE_HUNT_ARTIFACT,
		},
		{
			// Client event notebook provides the following in env.
			Env: ordereddict.NewDict().
				Set("ArtifactName", "Server.Monitor.Health").
				Set("ClientId", "C.123").
				Set("StartTime", "2025-07-14T23:59:53Z"),
			Args: ordereddict.NewDict(),
			Mode: MODE_EVENT_ARTIFACT,
		},

		// Pass parameters in notebook
		{
			// Client event notebook provides the following in env.
			Env: ordereddict.NewDict().
				Set("ArtifactName", "Server.Monitor.Health").
				Set("ClientId", "C.123").
				Set("StartTime", "2025-07-14T23:59:53Z"),
			Args: ordereddict.NewDict().
				Set("notebook_id", "N.123").
				Set("notebook_cell_id", "NC.1234"),
			Mode: MODE_NOTEBOOK,
		},

		// No Env
		{
			// Client flow notebook provides the following in env.
			Env: ordereddict.NewDict(),
			Args: ordereddict.NewDict().
				Set("artifact", "Generic.Client.Info").
				Set("client_id", "C.123").
				Set("flow_id", "F.123"),
			Mode: MODE_FLOW_ARTIFACT,
		},

		// Conflicting Env and Args - Args override.
		{
			// Client flow notebook provides the following in env.
			Env: ordereddict.NewDict().
				Set("ClientId", "C.876").
				Set("ArtifactName", "Server.Internal.Enrollment"),
			Args: ordereddict.NewDict().
				Set("artifact", "Generic.Client.Info").
				Set("client_id", "C.123").
				Set("flow_id", "F.123"),
			Mode: MODE_FLOW_ARTIFACT,
			Assertion: func(arg *SourcePluginArgs) bool {
				// Command line args trump env variables.
				return arg.ClientId == "C.123"
			},
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
		err = arg_parser.ExtractArgsWithContext(self.Ctx, scope, tc.Args, arg)
		assert.NoError(self.T(), err)

		err = arg.DetermineMode(self.Ctx, self.ConfigObj, scope, tc.Args)
		assert.NoError(self.T(), err)

		assert.Equal(self.T(), tc.Mode, arg.mode)

		if tc.Assertion != nil {
			assert.True(self.T(), tc.Assertion(arg))
		}
	}
}
