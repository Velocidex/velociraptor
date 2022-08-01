package file_store_test

import (
	"io/ioutil"
	"testing"

	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/file_store"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	file_store_api "www.velocidex.com/golang/velociraptor/file_store"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type FileStoreAccessorTestSuite struct {
	test_utils.TestSuite
}

func (self *FileStoreAccessorTestSuite) createFilestoreContents() {
	file_store_factory := file_store_api.GetFileStore(self.ConfigObj)
	path_spec := path_specs.NewSafeFilestorePath("subdir", "Foo").
		SetType(api.PATH_TYPE_FILESTORE_JSON)
	fd, err := file_store_factory.WriteFile(path_spec)
	assert.NoError(self.T(), err)
	fd.Write([]byte("hello world"))
	fd.Close()
}

func (self *FileStoreAccessorTestSuite) TestGlob() {
	self.createFilestoreContents()

	fs_accessor := file_store_accessor.NewFileStoreFileSystemAccessor(self.ConfigObj)

	globber := glob.NewGlobber()

	glob_expr, err := fs_accessor.ParsePath("/**")
	assert.NoError(self.T(), err)

	root_path, err := fs_accessor.ParsePath("fs:/")
	assert.NoError(self.T(), err)

	err = globber.Add(glob_expr)
	assert.NoError(self.T(), err)
	var returned []string

	scope := vql_subsystem.MakeScope()
	output_chan := globber.ExpandWithContext(
		self.Ctx, scope, self.ConfigObj,
		root_path, fs_accessor)
	for row := range output_chan {
		returned = append(returned, row.FullPath())
		if !row.IsDir() {
			fd, err := fs_accessor.OpenWithOSPath(row.OSPath())
			assert.NoError(self.T(), err)
			data, err := ioutil.ReadAll(fd)
			assert.NoError(self.T(), err)
			assert.Equal(self.T(), "hello world", string(data))
		}
	}

	assert.Equal(self.T(), 2, len(returned))
	goldie.Assert(self.T(), "TestGlob", json.MustMarshalIndent(returned))
}

func TestFileStoreAccessor(t *testing.T) {
	suite.Run(t, &FileStoreAccessorTestSuite{})
}
