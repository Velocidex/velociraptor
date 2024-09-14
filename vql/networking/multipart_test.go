package networking_test

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
)

type HTTPTestSuite struct {
	test_utils.TestSuite
}

func (self *HTTPTestSuite) TestMultipartUploadTest() {
	networking.BoundaryForTests = "AAAA"

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Env: ordereddict.NewDict().
			Set("Data1", "This is some data 1").
			Set("Data2", "This is Data2"),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	params := ordereddict.NewDict().
		Set("Baz", "Bar").
		Set("Foo", "Bar2")

	files := []*ordereddict.Dict{
		ordereddict.NewDict().
			Set("file", "MyFile.txt").
			Set("key", "file").
			Set("path", accessors.MustNewGenericOSPath("Data1")).
			Set("accessor", "scope"),

		ordereddict.NewDict().
			Set("file", "My Second File.txt").
			Set("key", "file").
			Set("path", accessors.MustNewGenericOSPath("Data2")).
			Set("accessor", "scope"),
	}

	golden := ordereddict.NewDict()
	uploader, err := networking.GetMultiPartReader(self.Ctx, scope, files, params)
	assert.NoError(self.T(), err)

	golden.Set("Content-Type", uploader.ContentType())
	golden.Set("Content-Length", uploader.ContentLength())

	data, err := ioutil.ReadAll(uploader.Reader())
	assert.NoError(self.T(), err)

	golden.Set("Uploaded Data", strings.Split(string(data), "\r\n"))
	golden.Set("Buffer Length", len(data))

	// Content length has to be the same as the total data
	assert.Equal(self.T(), len(data), uploader.ContentLength())

	goldie.Assert(self.T(), "TestMultipartUploadTest",
		json.MustMarshalIndent(golden))
}

func TestHTTPPlugins(t *testing.T) {
	suite.Run(t, &HTTPTestSuite{})
}
