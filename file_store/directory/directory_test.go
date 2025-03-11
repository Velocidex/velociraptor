package directory_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/tests"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
)

type DirectoryTestSuite struct {
	*tests.FileStoreTestSuite

	config_obj *config_proto.Config
	file_store *directory.DirectoryFileStore
}

func (self *DirectoryTestSuite) SetupTest() {
	dir, err := tempfile.TempDir("file_store_test")
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
	file_store_factory := directory.NewDirectoryFileStore(config_obj)

	file_store.OverrideFilestoreImplementation(config_obj, file_store_factory)

	suite.Run(t, &DirectoryTestSuite{
		FileStoreTestSuite: tests.NewFileStoreTestSuite(config_obj, file_store_factory),
		file_store:         file_store_factory,
		config_obj:         config_obj,
	})
}
