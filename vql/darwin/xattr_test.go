package darwin

import (
	"context"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/ivaxer/go-xattr"
	"github.com/stretchr/testify/suite"
	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"
)

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
	attr2 := "vr.test"
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

	xattr.Set(filepath, attr1, []byte("test-value"))
	xattr.Set(filepath, attr2, []byte("test-value"))

	self.Ctx = context.Background()
	sub_ctx, cancel := context.WithCancel(self.Ctx)
	defer cancel()

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

	for i, test := range testcases {
		defer scope.Close()
		ret := XAttrFunction{}.Call(sub_ctx, scope, ordereddict.NewDict().Set("filename", test.name).Set("attribute", test.attr))
		assert.Equal(self.T(), test.pass, (ret != vfilter.Null{}), "These two values should be the same. Test %d", i)
	}
}

func TestXAttrFunction(t *testing.T) {
	suite.Run(t, &XattrTestSuite{})
}
