package hunts

import (
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var testArtifacts = []string{`
name: Test.Artifact
`, `
name: Server.Audit.Logs
`}

type TestSuite struct {
	test_utils.TestSuite
	client_id, flow_id string
}

func (self *TestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.HuntDispatcher = true

	self.TestSuite.SetupTest()
}

var testCases = []struct {
	description string
	args        *ordereddict.Dict
}{
	{
		description: "simple hunt",
		args: ordereddict.NewDict().
			Set("description", "foo").
			Set("include_labels", "Label1").
			Set("artifacts", "Test.Artifact"),
	},
	{
		description: "exclude label hunt",
		args: ordereddict.NewDict().
			Set("description", "foo").
			Set("exclude_labels", "ExcludeLabel").
			Set("artifacts", "Test.Artifact"),
	},
	{
		description: "include label hunt",
		args: ordereddict.NewDict().
			Set("description", "foo").
			Set("include_labels", "IncludeLabel").
			Set("artifacts", "Test.Artifact"),
	},
	{
		description: "include and exclude label hunt",
		args: ordereddict.NewDict().
			Set("description", "foo").
			Set("exclude_labels", "ExcludeLabel").
			Set("include_labels", "IncludeLabel").
			Set("artifacts", "Test.Artifact"),
	},
	{
		description: "os hunt",
		args: ordereddict.NewDict().
			Set("description", "foo").
			Set("os", "windows").
			Set("artifacts", "Test.Artifact"),
	},
	{
		description: "os and label hunt",
		args: ordereddict.NewDict().
			Set("description", "foo").
			Set("os", "windows").
			Set("include_labels", "IncludeLabel").
			Set("artifacts", "Test.Artifact"),
	},
}

func (self *TestSuite) TestCreateHunt() {
	result := ordereddict.NewDict()
	hunt_dispatcher.SetHuntIdForTests("H.1234")
	Clock := &utils.MockClock{}
	Clock.Set(time.Unix(100, 0))

	hunt_dispatcher.Clock = Clock

	closer := utils.MockTime(hunt_dispatcher.Clock)
	defer closer()

	repository := self.LoadArtifacts(testArtifacts...)
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Repository: repository,
		Logger: logging.NewPlainLogger(
			self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)
	scope := manager.BuildScope(builder)
	defer scope.Close()

	plugin := &ScheduleHuntFunction{}
	for _, test_case := range testCases {
		result.Set(test_case.description, plugin.Call(
			self.Ctx, scope, test_case.args))
	}
	goldie.Assert(self.T(), "TestCreateHunt",
		json.MustMarshalIndent(result))
}

func TestHuntPlugin(t *testing.T) {
	suite.Run(t, &TestSuite{
		client_id: "C.123",
		flow_id:   "F.123",
	})
}
