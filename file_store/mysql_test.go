package file_store

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type MysqlTestSuite struct {
	suite.Suite

	config_obj *config_proto.Config
	filestore  FileStore
}

func (self *MysqlTestSuite) SetupTest() {
	// Drop and initialize the database to start a new test.
	conn_string := fmt.Sprintf("%s:%s@tcp(%s)/",
		self.config_obj.Datastore.MysqlUsername,
		self.config_obj.Datastore.MysqlPassword,
		self.config_obj.Datastore.MysqlServer)

	// Make sure our database is not the same as the datastore
	// tests or else we will trash over them.
	self.config_obj.Datastore.MysqlDatabase += "fs"

	db, err := sql.Open("mysql", conn_string)
	assert.NoError(self.T(), err)

	db.Exec(fmt.Sprintf("drop database `%v`",
		self.config_obj.Datastore.MysqlDatabase))

	defer db.Close()

	initializeDatabase(self.config_obj)

	self.filestore, err = NewSqlFileStore(self.config_obj)
	assert.NoError(self.T(), err)
}

func (self *MysqlTestSuite) TestListChildrenIntermediateDirs() {
	fd, err := self.filestore.WriteFile("/a/b/c/d/Foo")
	assert.NoError(self.T(), err)
	defer fd.Close()

	infos, err := self.filestore.ListDirectory("/a")
	assert.NoError(self.T(), err)

	names := []string{}
	for _, info := range infos {
		names = append(names, info.Name())
	}

	sort.Strings(names)
	assert.Equal(self.T(), names, []string{"b"})
}

func (self *MysqlTestSuite) TestListChildren() {
	filename := "/a/b"
	fd, err := self.filestore.WriteFile(path.Join(filename, "Foo"))
	assert.NoError(self.T(), err)
	defer fd.Close()

	fd, err = self.filestore.WriteFile(path.Join(filename, "Bar"))
	assert.NoError(self.T(), err)
	defer fd.Close()

	fd, err = self.filestore.WriteFile(path.Join(filename, "Bar", "Baz"))
	assert.NoError(self.T(), err)
	defer fd.Close()

	infos, err := self.filestore.ListDirectory(filename)
	assert.NoError(self.T(), err)

	names := []string{}
	for _, info := range infos {
		names = append(names, info.Name())
	}

	sort.Strings(names)
	assert.Equal(self.T(), names, []string{"Bar", "Foo"})

	names = nil
	err = self.filestore.Walk(filename, func(path string, info os.FileInfo, err error) error {
		names = append(names, path)
		return nil
	})
	assert.NoError(self.T(), err)

	sort.Strings(names)
	assert.Equal(self.T(), names, []string{
		"/a/b/Bar",
		"/a/b/Bar/Baz",
		"/a/b/Foo"})
}

func (self *MysqlTestSuite) TestFileReadWrite() {
	fd, err := self.filestore.WriteFile("/test/foo")
	assert.NoError(self.T(), err)
	defer fd.Close()

	// Write some data.
	err = fd.Append([]byte("Some data"))
	assert.NoError(self.T(), err)

	// Check that size is incremeented.
	size, err := fd.Size()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(9), size)

	err = fd.Append([]byte("MORE data"))
	assert.NoError(self.T(), err)

	buff := make([]byte, 6)
	reader, err := self.filestore.ReadFile("/test/foo")
	assert.NoError(self.T(), err)
	defer reader.Close()

	n, err := reader.Read(buff)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), n, len(buff))
	assert.Equal(self.T(), "Some d", string(buff))

	n, err = reader.Read(buff)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), n, len(buff))
	assert.Equal(self.T(), "ataMOR", string(buff))

	// Over read past the end.
	buff = make([]byte, 60)
	n, err = reader.Read(buff)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), n, 6)
	assert.Equal(self.T(), "E data", string(buff[:n]))

	// Read at EOF - gives an EOF and 0 byte read.
	n, err = reader.Read(buff)
	assert.Equal(self.T(), err, io.EOF)
	assert.Equal(self.T(), n, 0)

	// Write some data.
	err = fd.Append([]byte("EXTRA EXTRA"))
	assert.NoError(self.T(), err)

	// New read picks the new data.
	n, err = reader.Read(buff)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), n, 11)
	assert.Equal(self.T(), "EXTRA EXTRA", string(buff[:n]))

	// Seek to middle of first chunk and read some data.
	_, err = reader.Seek(2, os.SEEK_SET)
	assert.NoError(self.T(), err)

	buff = make([]byte, 6)
	n, err = reader.Read(buff)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), n, len(buff))
	assert.Equal(self.T(), "me dat", string(buff[:n]))

	// Seek to no man's land
	_, err = reader.Seek(200, os.SEEK_SET)
	assert.NoError(self.T(), err)

	// Reading past the end of file should produce empty data.
	n, err = reader.Read(buff)
	assert.Equal(self.T(), err, io.EOF)
	assert.Equal(self.T(), n, 0)

	// Reopenning the file should give the right size.
	size, err = fd.Size()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(29), size)

	fd, err = self.filestore.WriteFile("/test/foo")
	assert.NoError(self.T(), err)
	size, err = fd.Size()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(29), size)
}

func TestMysqlDatabase(t *testing.T) {
	// If a local testing mysql server is configured we can run
	// this test, otherwise skip it.
	config_obj, err := config.LoadConfig("../datastore/test_data/mysql.config.yaml")
	if err != nil {
		return
	}

	suite.Run(t, &MysqlTestSuite{
		config_obj: config_obj,
	})
}
