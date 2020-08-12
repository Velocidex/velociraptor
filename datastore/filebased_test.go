package datastore

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type FilebasedTestSuite struct {
	BaseTestSuite
}

func TestFilebasedDatabase(t *testing.T) {
	dir, err := ioutil.TempDir("", "datastore_test")
	assert.NoError(t, err)

	defer os.RemoveAll(dir) // clean up

	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.FilestoreDirectory = dir
	config_obj.Datastore.Location = dir

	suite.Run(t, &FilebasedTestSuite{BaseTestSuite{
		datastore: &FileBaseDataStore{
			clock: vtesting.RealClock{},
		},
		config_obj: config_obj,
	}})
}
