package server

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var linkTestCases = []struct {
	Name string
	Args *ordereddict.Dict
}{{
	Name: "Link to hunt",
	Args: ordereddict.NewDict().
		Set("hunt_id", "H.1234").
		Set("text", "Link To Hunt"),
}, {
	Name: "Link to flow",
	Args: ordereddict.NewDict().
		Set("client_id", "C.123").
		Set("flow_id", "F.123").
		Set("text", "Link To Flow"),
}, {
	Name: "Link to client",
	Args: ordereddict.NewDict().
		Set("client_id", "C.123").
		Set("text", "Link To Client"),
}, {
	Name: "Link to event artifact",
	Args: ordereddict.NewDict().
		Set("client_id", "C.123").
		Set("artifact", "Custom.Artifact.Name").
		Set("text", "Link To event artifact"),
}, {
	Name: "Link to artifact",
	Args: ordereddict.NewDict().
		Set("artifact", "Custom.Artifact.Name").
		Set("text", "Link To Artifact"),
}, {
	Name: "Link to upload",
	Args: ordereddict.NewDict().
		Set("upload", ordereddict.NewDict().
			Set("Components", []string{
				"notebooks", "N.123", "NC.123",
				"uploads", "data", "Hello"})).
		Set("text", "Link To upload"),
}, {
	Name: "Link to create flow",
	Args: ordereddict.NewDict().
		Set("client_id", "C.123").
		Set("flow_id", "new").
		Set("artifact", "Demo.Plugins.GUI").
		Set("parameters", ordereddict.NewDict().
			// Test the / is escaped in the params section
			Set("YaraRule", "rule X/Y { ... }")).
		Set("text", "Create new collection"),
}, {
	Name: "Link to create flow raw",
	Args: ordereddict.NewDict().
		Set("client_id", "C.123").
		Set("flow_id", "new").
		Set("artifact", "Demo.Plugins.GUI").
		Set("parameters", ordereddict.NewDict().
			// Test the / is escaped in the params section
			// space must be escaped to %20
			// + must be escaped
			Set("YaraRule", "rule X+/Y { ... }")).
		Set("raw", true),
}, {
	Name: "Link to create a new notebook",
	Args: ordereddict.NewDict().
		Set("notebook_id", "new").
		Set("artifact", "Notebook.Demo").
		Set("parameters", ordereddict.NewDict().
			Set("name", "My Notebook").
			Set("description", "A lovely notebook").
			Set("AnInteger", "76")).
		Set("text", "Link To Notebook"),
}}

type LinksTestSuite struct {
	test_utils.TestSuite
}

func (self *LinksTestSuite) TestLinks() {
	f := &LinkToFunction{}

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	golden := ordereddict.NewDict()
	for _, tc := range linkTestCases {
		golden.Set(tc.Name, ordereddict.NewDict().
			Set("Args", tc.Args).
			Set("Result", f.Call(self.Ctx, scope,
				tc.Args)))
	}

	goldie.Assert(self.T(), "TestLinks",
		json.MustMarshalIndent(golden))
}

func TestLinkToPlugin(t *testing.T) {
	suite.Run(t, &LinksTestSuite{})
}
