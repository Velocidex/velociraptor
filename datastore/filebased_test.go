package datastore_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
)

type FilebasedTestSuite struct {
	BaseTestSuite
	dirname string
}

func (self FilebasedTestSuite) DumpDirectory() {
	filepath.Walk(self.dirname, func(path string,
		info os.FileInfo, err error) error {
		if !info.IsDir() {
			fmt.Printf("%v: %v\n", path, info.Size())
		}
		return nil
	})
}

func fillUpDisk(writer io.Writer) {
	pad := make([]byte, 1024*1024)
	for {
		n, err := writer.Write(pad)
		if n < len(pad) || err != nil {
			// Start writing small buffers to really fill up the disk.
			for {
				n, err := writer.Write(pad[:10])
				if n < len(pad) || err != nil {
					return
				}
			}
		}
	}
}

// Test how this filestore behaves when the disk is full.
// To setup the test create a small filesystem
// dd if=/dev/zero of=/tmp/small.dd count=10 bs=1M
// losetup /dev/loop6 /tmp/small.dd
// mke2fs /dev/loop6
// mount /dev/loop6 /tmp/small_partition/
func (self FilebasedTestSuite) TestFullDiskErrors() {
	self.T().Skip("Manual setup required")

	small_partition := "/tmp/small_partition"
	sample_obj := &crypto_proto.VeloMessage{Source: "Server"}
	obj_path := path_specs.NewUnsafeDatastorePath("test")
	self.config_obj.Datastore.FilestoreDirectory = small_partition
	self.config_obj.Datastore.Location = small_partition
	self.config_obj.Datastore.MinAllowedFileSpaceMb = 1

	pad_path := path_specs.NewUnsafeDatastorePath("pad.dd")
	self.datastore.DeleteSubject(self.config_obj, pad_path)

	// Check that there is enough space
	available, err := datastore.AvailableDiskSpace(self.datastore,
		self.config_obj)
	assert.NoError(self.T(), err)
	assert.True(self.T(), available > 0)

	err = self.datastore.SetSubject(self.config_obj,
		obj_path, sample_obj)
	assert.NoError(self.T(), err)

	// Fill the disk now
	fd, err := os.OpenFile(
		pad_path.AsDatastoreFilename(self.config_obj),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
	assert.NoError(self.T(), err)
	fillUpDisk(fd)
	fd.Close()

	// Check that the disk is full
	available, err = datastore.AvailableDiskSpace(
		self.datastore, self.config_obj)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), uint64(0), available)

	// Writes should be blocked now.
	sample_obj.Source = strings.Repeat("TestString", 1000)
	err = self.datastore.SetSubject(self.config_obj,
		obj_path, sample_obj)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "Insufficient")

	// The old file is not touched
	test_obj := &crypto_proto.VeloMessage{}
	err = self.datastore.GetSubject(self.config_obj, obj_path, test_obj)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), test_obj.Source, "Server")
}

func (self FilebasedTestSuite) TestGetSubjectOfEmptyFileIsError() {
	path := path_specs.NewUnsafeDatastorePath("test")

	// Create an empty file
	fd, err := os.OpenFile(
		path.AsDatastoreFilename(self.config_obj), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
	assert.NoError(self.T(), err)
	fd.Close()

	// Try to read from it. This is an error because the file is invalid json.
	read_message := &crypto_proto.VeloMessage{}
	err = self.datastore.GetSubject(self.config_obj, path, read_message)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "Invalid file")
}

func (self FilebasedTestSuite) TestSetGetSubjectWithEscaping() {
	self.BaseTestSuite.TestSetGetSubjectWithEscaping()
	// self.DumpDirectory()
}

func (self FilebasedTestSuite) TestSetGetJSON() {
	self.BaseTestSuite.TestSetGetJSON()
	// self.DumpDirectory()
}

func (self *FilebasedTestSuite) SetupTest() {
	var err error
	self.dirname, err = tempfile.TempDir("datastore_test")
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
			datastore: &datastore.FileBaseDataStore{},
		},
	})
}
