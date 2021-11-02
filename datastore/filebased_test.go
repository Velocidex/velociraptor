package datastore

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
)

type FilebasedTestSuite struct {
	BaseTestSuite
	dirname string
}

func (self FilebasedTestSuite) DumpDirectory() {
	filepath.Walk(self.dirname, func(path string,
		info fs.FileInfo, err error) error {
		if !info.IsDir() {
			fmt.Printf("%v: %v\n", path, info.Size())
		}
		return nil
	})
}

func (self FilebasedTestSuite) TestSetGetSubjectWithEscaping() {
	self.BaseTestSuite.TestSetGetSubjectWithEscaping()
	self.DumpDirectory()
}

func (self FilebasedTestSuite) TestSetGetJSON() {
	self.BaseTestSuite.TestSetGetJSON()
	self.DumpDirectory()
}

func (self *FilebasedTestSuite) SetupTest() {
	var err error
	self.dirname, err = ioutil.TempDir("", "datastore_test")
	assert.NoError(self.T(), err)

	self.config_obj = config.GetDefaultConfig()
	self.config_obj.Datastore.FilestoreDirectory = self.dirname
	self.config_obj.Datastore.Location = self.dirname
	self.BaseTestSuite.config_obj = self.config_obj
}

func (self FilebasedTestSuite) TearDownTest() {
	os.RemoveAll(self.dirname) // clean up
}

func TestFilebasedDatabase(t *testing.T) {
	suite.Run(t, &FilebasedTestSuite{
		BaseTestSuite: BaseTestSuite{
			datastore: &FileBaseDataStore{},
		},
	})
}
