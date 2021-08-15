package directory

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/tests"
)

type DirectoryTestSuite struct {
	*tests.FileStoreTestSuite

	config_obj *config_proto.Config
	file_store *DirectoryFileStore
}

func (self *DirectoryTestSuite) SetupTest() {
	dir, err := ioutil.TempDir("", "file_store_test")
	assert.NoError(self.T(), err)

	self.config_obj.Datastore.FilestoreDirectory = dir
	self.config_obj.Datastore.Location = dir
}

func (self *DirectoryTestSuite) TearDownTest() {
	// clean up
	os.RemoveAll(self.config_obj.Datastore.FilestoreDirectory)
}

func TestDirectoryFileStore(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	file_store := NewDirectoryFileStore(config_obj)
	suite.Run(t, &DirectoryTestSuite{
		FileStoreTestSuite: tests.NewFileStoreTestSuite(config_obj, file_store),
		file_store:         file_store,
		config_obj:         config_obj,
	})
}
