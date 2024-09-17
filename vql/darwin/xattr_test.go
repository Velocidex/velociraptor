//go:build linux || darwin

package darwin

import (
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var ()

type XattrTestSuite struct {
	test_utils.TestSuite
}

func (self *XattrTestSuite) TestXAttr() {
	file, err := os.CreateTemp("", "")
	assert.NoError(self.T(), err, "xattr: failed to create test file")
	filepath := file.Name()
	file.Close()
	defer os.Remove(filepath)

	testcases := []struct {
		name     string
		filename string
		attr     []string
		pass     bool
	}{
		{
			name:     "List All attributes",
			filename: filepath,
			attr:     []string{},
			pass:     false,
		},
		{
			name:     "Get one specific attribute",
			filename: filepath,
			attr:     []string{"vr.test"},
			pass:     true,
		},
		{
			name:     "Get both attributes by name",
			filename: filepath,
			attr:     []string{"vr.test", "vr.test2"},
			pass:     true,
		},
		{
			name:     "Get one attribute present and one not available.",
			filename: filepath,
			attr:     []string{"vr.test", "invalid.test"},
			pass:     true,
		},
		{
			name:     "Get only unavailable attribute",
			filename: filepath,
			attr:     []string{"invalid.test"},
			pass:     false,
		},
	}

	err = Set(filepath, "vr.test", []byte("test.value"))
	assert.NoError(self.T(), err)

	err = Set(filepath, "vr.test2", []byte("test.value"))
	assert.NoError(self.T(), err)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger: logging.NewPlainLogger(self.ConfigObj,
			&logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	ret := ordereddict.NewDict()
	for _, test := range testcases {
		res := XAttrFunction{}.Call(
			self.Ctx, scope,
			ordereddict.NewDict().
				Set("filename", test.filename).
				Set("attribute", test.attr))
		ret.Set(test.name, res)

	}
	goldie.Assert(self.T(), "TestXAttr", json.MustMarshalIndent(ret))
}

func TestXAttrFunction(t *testing.T) {
	suite.Run(t, &XattrTestSuite{})
}
