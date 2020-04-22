package api

import (
	"fmt"
	"io"
	"os"
	"path"
	"sort"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// An abstract test suite to ensure file store implementations all
// comply with the API.
type FileStoreTestSuite struct {
	suite.Suite

	config_obj *config_proto.Config
	filestore  FileStore
}

func NewFileStoreTestSuite(config_obj *config_proto.Config,
	filestore FileStore) *FileStoreTestSuite {
	return &FileStoreTestSuite{
		config_obj: config_obj,
		filestore:  filestore,
	}
}

func (self *FileStoreTestSuite) TestListChildrenIntermediateDirs() {
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

func (self *FileStoreTestSuite) TestListChildren() {
	filename := "/a/b"
	fd, err := self.filestore.WriteFile(path.Join(filename, "Foo.txt"))
	assert.NoError(self.T(), err)
	defer fd.Close()

	fd, err = self.filestore.WriteFile(path.Join(filename, "Bar.txt"))
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
	assert.Equal(self.T(), names, []string{"Bar", "Bar.txt", "Foo.txt"})

	names = nil
	err = self.filestore.Walk(filename, func(path string, info os.FileInfo, err error) error {
		// Ignore directories as they are not important.
		if !info.IsDir() {
			names = append(names, path)
		}
		return nil
	})
	assert.NoError(self.T(), err)

	sort.Strings(names)
	fmt.Println(names)
	assert.Equal(self.T(), names, []string{
		"/a/b/Bar.txt",
		"/a/b/Bar/Baz",
		"/a/b/Foo.txt"})
}

func (self *FileStoreTestSuite) TestFileReadWrite() {
	fd, err := self.filestore.WriteFile("/test/foo")
	assert.NoError(self.T(), err)
	defer fd.Close()

	// Write some data.
	_, err = fd.Write([]byte("Some data"))
	assert.NoError(self.T(), err)

	// Check that size is incremeented.
	size, err := fd.Size()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(9), size)

	_, err = fd.Write([]byte("MORE data"))
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
	_, err = fd.Write([]byte("EXTRA EXTRA"))
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

type QueueManagerTestSuite struct {
	suite.Suite

	config_obj *config_proto.Config
	manager    QueueManager
}

func (self *QueueManagerTestSuite) TestPush() {
	payload := []byte("[{\"foo\":1},{\"foo\":2}]")

	err := self.manager.Push("System.Hunt.Participation", "C.123", payload)
	assert.NoError(self.T(), err)
}

func NewQueueManagerTestSuite(config_obj *config_proto.Config,
	manager QueueManager) *QueueManagerTestSuite {
	return &QueueManagerTestSuite{
		config_obj: config_obj,
		manager:    manager,
	}
}
