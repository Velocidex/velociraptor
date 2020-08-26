//nolint

package api

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"sort"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

type Debugger interface {
	Debug()
}

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

	// Walk non existant directory just returns no results.
	names = nil
	err = self.filestore.Walk(filename+"/nonexistant", func(path string, info os.FileInfo, err error) error {
		names = append(names, path)
		return nil
	})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(names), 0)
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
	_, err = reader.Seek(2, io.SeekStart)
	assert.NoError(self.T(), err)

	buff = make([]byte, 6)
	n, err = reader.Read(buff)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), n, len(buff))
	assert.Equal(self.T(), "me dat", string(buff[:n]))

	// Seek to no man's land
	_, err = reader.Seek(200, io.SeekStart)
	assert.NoError(self.T(), err)

	// Reading past the end of file should produce empty data.
	n, err = reader.Read(buff)
	assert.Equal(self.T(), err, io.EOF)
	assert.Equal(self.T(), n, 0)

	// Seek to the last chunk and read a large buffer.
	_, err = reader.Seek(25, io.SeekStart)
	assert.NoError(self.T(), err)

	// Reading past the end of file should produce empty data.
	buff = make([]byte, 1000)
	n, err = reader.Read(buff)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), n, 4)

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
	file_store FileStore
}

func (self *QueueManagerTestSuite) Debug() {
	switch t := self.manager.(type) {
	case Debugger:
		t.Debug()
	}
}

func (self *QueueManagerTestSuite) FilestoreGet(path string) string {
	fd, _ := self.file_store.ReadFile(path)
	value, _ := ioutil.ReadAll(fd)
	return string(value)
}

func (self *QueueManagerTestSuite) TestPush() {
	artifact_name := "System.Hunt.Participation"

	payload := []*ordereddict.Dict{
		ordereddict.NewDict().Set("foo", 1),
		ordereddict.NewDict().Set("foo", 2)}

	output, cancel := self.manager.Watch(artifact_name)
	defer cancel()

	err := self.manager.PushEventRows(
		MockPathManager{"log_path", artifact_name}, payload)

	assert.NoError(self.T(), err)

	for row := range output {
		value, _ := row.Get("foo")
		v, _ := utils.ToInt64(value)
		if v == int64(2) {
			break
		}

		ts, _ := row.Get("_ts")
		assert.NotNil(self.T(), ts)
	}

	// Make sure the manager wrote the event to the filestore as well.
	assert.Contains(self.T(), self.FilestoreGet("log_path"), "foo")
}

func NewQueueManagerTestSuite(
	config_obj *config_proto.Config,
	manager QueueManager,
	file_store FileStore) *QueueManagerTestSuite {
	return &QueueManagerTestSuite{
		config_obj: config_obj,
		manager:    manager,
		file_store: file_store,
	}
}

type MockPathManager struct {
	Path         string
	ArtifactName string
}

func (self MockPathManager) GetPathForWriting() (string, error) {
	return self.Path, nil
}

func (self MockPathManager) GetQueueName() string {
	return self.ArtifactName
}

func (self MockPathManager) GeneratePaths(ctx context.Context) <-chan *ResultSetFileProperties {
	output := make(chan *ResultSetFileProperties)

	go func() {
		defer close(output)

	}()

	return output
}
