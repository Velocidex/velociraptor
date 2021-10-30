package datastore

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
)

type MemcacheFileTestSuite struct {
	BaseTestSuite

	dirname string
}

func (self *MemcacheFileTestSuite) SetupTest() {
	// Clear the cache between runs
	self.datastore.(*MemcacheFileDataStore).Clear()

	// Make a tempdir
	var err error
	self.dirname, err = ioutil.TempDir("", "datastore_test")
	assert.NoError(self.T(), err)

	self.config_obj = config.GetDefaultConfig()
	self.config_obj.Datastore.FilestoreDirectory = self.dirname
	self.config_obj.Datastore.Location = self.dirname
	self.BaseTestSuite.config_obj = self.config_obj
}

func (self MemcacheFileTestSuite) TearDownTest() {
	os.RemoveAll(self.dirname) // clean up
}

func (self MemcacheFileTestSuite) TestSetGetSubject() {
	self.BaseTestSuite.TestSetGetSubject()

	file_based_imp.Debug(self.config_obj)
}

func TestMemCacheFileDatastore(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "MemcacheFileDatastore"

	suite.Run(t, &MemcacheFileTestSuite{BaseTestSuite: BaseTestSuite{
		datastore:  NewMemcacheFileDataStore(),
		config_obj: config_obj,
	}})
}
