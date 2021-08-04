package directory

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/tests"
)

func TestDirectoryFileStore(t *testing.T) {
	dir, err := ioutil.TempDir("", "file_store_test")
	assert.NoError(t, err)

	defer os.RemoveAll(dir) // clean up

	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.FilestoreDirectory = dir + "/"
	config_obj.Datastore.Location = dir + "/"

	file_store := NewDirectoryFileStore(config_obj)
	suite.Run(t, tests.NewFileStoreTestSuite(config_obj, file_store))
}
