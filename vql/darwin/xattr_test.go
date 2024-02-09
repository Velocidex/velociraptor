package darwin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/ivaxer/go-xattr"
	"github.com/stretchr/testify/suite"
	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type XattrTestSuite struct {
	test_utils.TestSuite
}

func (self *XattrTestSuite) TestXAttr() {
	filepath := "/tmp/xattr.test"
	attr1 := "vr.test"
	attr2 := "vr.test"

	_, err := os.Create(filepath)
	if err != nil {
		self.T().Errorf("xattr: failed to create test file: %s", err)
		return
	}
	defer os.Remove(filepath)

	xattr.Set(filepath, attr1, []byte("test-value"))
	xattr.Set(filepath, attr2, []byte("test-value"))

	ctx := context.Background()
	sub_ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
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
			attr: []string{"invalud.test"},
			pass: false,
		},
	}

	for i, test := range testcases {
		defer scope.Close()
		ret := XAttrFunction{}.Call(sub_ctx, scope, ordereddict.NewDict().Set("filename", test.name).Set("attribute", test.attr))
		if ret == nil {
			if test.pass {
				self.T().Errorf("xattr: test %d: Got %t, espected %t", i, false, test.pass)
			}
		} else {
			if !test.pass {
				self.T().Errorf("xattr: test %d: Got %t, espected %t", i, true, test.pass)
			}
		}
	}
}

func TestXAttrFunction(t *testing.T) {
	suite.Run(t, &XattrTestSuite{})
}
