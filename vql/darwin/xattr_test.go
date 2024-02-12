//go:build !windows
// +build !windows

package darwin

import (
	"fmt"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/ivaxer/go-xattr"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
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

	attr1 := "vr.test"
	attr2 := "vr.test2"
	value := "test.value"

	testcases := []struct {
		name string
		attr []string
		pass bool
	}{
		{
			name: filepath,
			attr: []string{},
			pass: false,
		},
		{
			name: filepath,
			attr: []string{attr1},
			pass: true,
		},
		{
			name: filepath,
			attr: []string{attr1, attr2},
			pass: true,
		},
		{
			name: filepath,
			attr: []string{attr1, "invalid.test"},
			pass: true,
		},
		{
			name: filepath,
			attr: []string{"invalid.test"},
			pass: false,
		},
	}

	xattr.Set(filepath, attr1, []byte(value))
	xattr.Set(filepath, attr2, []byte(value))

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
	for i, test := range testcases {
		ret.Set(fmt.Sprintf("Case #%d", i), XAttrFunction{}.Call(
			self.Ctx, scope,
			ordereddict.NewDict().
				Set("filename", test.name).
				Set("attribute", test.attr)))

	}
	goldie.Assert(self.T(), "xattrReturnCheck", json.MustMarshalIndent(ret))
}

func TestXAttrFunction(t *testing.T) {
	suite.Run(t, &XattrTestSuite{})
}
