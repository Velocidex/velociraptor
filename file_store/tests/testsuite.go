//nolint

package tests

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
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
	filestore  api.FileStore
}

func NewFileStoreTestSuite(config_obj *config_proto.Config,
	filestore api.FileStore) *FileStoreTestSuite {
	return &FileStoreTestSuite{
		config_obj: config_obj,
		filestore:  filestore,
	}
}

func (self *FileStoreTestSuite) TestListChildrenIntermediateDirs() {
	components := path_specs.NewSafeFilestorePath("a", "b", "c", "d", "Foo")
	fd, err := self.filestore.WriteFile(components)
	assert.NoError(self.T(), err)
	fd.Close()

	infos, err := self.filestore.ListDirectory(
		path_specs.NewSafeFilestorePath("a"))
	assert.NoError(self.T(), err)

	names := []string{}
	for _, info := range infos {
		names = append(names, info.Name())
	}

	sort.Strings(names)
	assert.Equal(self.T(), names, []string{"b"})
}

func (self *FileStoreTestSuite) TestListChildrenSameNameDifferentTypes() {
	path_spec := path_specs.NewSafeFilestorePath("subdir", "Foo").
		SetType(api.PATH_TYPE_FILESTORE_JSON)
	fd, err := self.filestore.WriteFile(path_spec)
	assert.NoError(self.T(), err)
	fd.Close()

	fd, err = self.filestore.WriteFile(path_spec.
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
	assert.NoError(self.T(), err)
	fd.Close()

	fd, err = self.filestore.WriteFile(path_spec.AddChild("dir", "value").
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
	assert.NoError(self.T(), err)
	fd.Close()

	fd, err = self.filestore.WriteFile(path_spec.AddChild("dir2", "value").
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
	assert.NoError(self.T(), err)
	fd.Close()

	infos, err := self.filestore.ListDirectory(
		path_specs.NewSafeFilestorePath("subdir"))
	assert.NoError(self.T(), err)

	names := []string{}
	for _, info := range infos {
		names = append(names, info.Name())
	}

	sort.Strings(names)
	// One for the directory, one for the JSON and one for the JSON
	// index
	assert.Equal(self.T(), names, []string{"Foo", "Foo", "Foo"})
}

// List children recovers child's type based on extensions.
func (self *FileStoreTestSuite) TestListChildrenWithTypes() {

	for idx, t := range []api.PathType{
		api.PATH_TYPE_FILESTORE_JSON_INDEX,
		api.PATH_TYPE_FILESTORE_JSON,
		api.PATH_TYPE_FILESTORE_JSON_TIME_INDEX,

		// Used to write sparse indexes
		api.PATH_TYPE_FILESTORE_SPARSE_IDX,

		// Used to write zip files in the download folder.
		api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP,
		api.PATH_TYPE_FILESTORE_DOWNLOAD_REPORT,

		// TMP files
		api.PATH_TYPE_FILESTORE_TMP,
		api.PATH_TYPE_FILESTORE_LOCK,
		api.PATH_TYPE_FILESTORE_CSV,

		// Used for artifacts
		api.PATH_TYPE_FILESTORE_YAML,

		api.PATH_TYPE_FILESTORE_ANY,
	} {
		filename := path_specs.NewSafeFilestorePath(
			"a", fmt.Sprintf("b%v", idx)).SetType(t)

		fd, err := self.filestore.WriteFile(filename.AddChild("Foo.txt"))
		assert.NoError(self.T(), err)
		fd.Close()

		infos, err := self.filestore.ListDirectory(filename)
		assert.NoError(self.T(), err)

		assert.Equal(self.T(), 1, len(infos))

		// The type should be correct.
		assert.Equal(self.T(), t, infos[0].PathSpec().Type())

		// The extension should be correctly stripped so the
		// filename is roundtripped.
		assert.Equal(self.T(), infos[0].Name(), "Foo.txt")

		// Now check walk
		path_specs := []api.FSPathSpec{}
		err = api.Walk(self.filestore, filename, func(
			path api.FSPathSpec, info os.FileInfo) error {
			// Ignore directories as they are not important.
			if !info.IsDir() {
				path_specs = append(path_specs, path)
			}
			return nil
		})
		assert.NoError(self.T(), err)

		assert.Equal(self.T(), 1, len(path_specs))

		// The type should be correct.
		assert.Equal(self.T(), t, path_specs[0].Type())
	}
}

func (self *FileStoreTestSuite) TestListDirectory() {
	filename := path_specs.NewSafeFilestorePath("a", "b")
	fd, err := self.filestore.WriteFile(filename.AddChild("Foo.txt"))
	assert.NoError(self.T(), err)
	fd.Close()

	fd, err = self.filestore.WriteFile(filename.AddChild("Bar.txt"))
	assert.NoError(self.T(), err)
	fd.Close()

	fd, err = self.filestore.WriteFile(filename.AddChild("Bar", "Baz"))
	assert.NoError(self.T(), err)
	fd.Close()

	infos, err := self.filestore.ListDirectory(filename)
	assert.NoError(self.T(), err)

	names := []string{}
	for _, info := range infos {
		names = append(names, info.Name())
	}

	sort.Strings(names)
	assert.Equal(self.T(), names, []string{"Bar", "Bar.txt", "Foo.txt"})

	names = nil
	err = api.Walk(self.filestore, filename, func(
		path api.FSPathSpec, info os.FileInfo) error {
		names = append(names, path.AsClientPath())
		return nil
	})
	assert.NoError(self.T(), err)

	sort.Strings(names)
	// AsClientPath() restores the extension.
	assert.Equal(self.T(), []string{
		"/a/b/Bar.txt.json",
		"/a/b/Bar/Baz.json",
		"/a/b/Foo.txt.json"}, names)

	// Walk non existent directory just returns no results.
	names = nil
	err = api.Walk(self.filestore, filename.AddChild("nonexistant"),
		func(path api.FSPathSpec, info os.FileInfo) error {
			names = append(names, path.AsFilestoreFilename(
				self.config_obj))
			return nil
		})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(names), 0)
}

func (self *FileStoreTestSuite) TestFileReadWrite() {
	filename := path_specs.NewSafeFilestorePath("test", "foo")
	fd, err := self.filestore.WriteFile(filename)
	assert.NoError(self.T(), err)

	// Write some data.
	_, err = fd.Write([]byte("Some data"))
	assert.NoError(self.T(), err)

	// Check that size is incremeented.
	size, err := fd.Size()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(9), size)

	_, err = fd.Write([]byte("MORE data"))
	assert.NoError(self.T(), err)
	fd.Close()

	buff := make([]byte, 6)
	reader, err := self.filestore.ReadFile(filename)
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

	// Write some more data to the end of the file.
	fd, err = self.filestore.WriteFile(filename)
	assert.NoError(self.T(), err)
	_, err = fd.Write([]byte("EXTRA EXTRA"))
	assert.NoError(self.T(), err)
	fd.Close()

	// New read picks the new data.
	n, err = reader.Read(buff)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), n, 11)
	assert.Equal(self.T(), "EXTRA EXTRA", string(buff[:n]))

	// Seek to middle of first chunk and read within first chunk.
	_, err = reader.Seek(2, io.SeekStart)
	assert.NoError(self.T(), err)

	buff = make([]byte, 2)
	n, err = reader.Read(buff)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), n, len(buff))
	assert.Equal(self.T(), "me", string(buff[:n]))

	// Seek to middle of first chunk and read some data across to next chunk.
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
	fd, err = self.filestore.WriteFile(filename)
	assert.NoError(self.T(), err)
	size, err = fd.Size()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(29), size)
	fd.Close()
}

type QueueManagerTestSuite struct {
	suite.Suite

	config_obj *config_proto.Config
	manager    api.QueueManager
	file_store api.FileStore
}

func (self *QueueManagerTestSuite) Debug() {
	switch t := self.manager.(type) {
	case Debugger:
		t.Debug()
	}
}

func (self *QueueManagerTestSuite) FilestoreGet(path api.FSPathSpec) string {
	fd, err := self.file_store.ReadFile(path)
	assert.NoError(self.T(), err)
	value, err := ioutil.ReadAll(fd)
	assert.NoError(self.T(), err)
	return string(value)
}

func (self *QueueManagerTestSuite) TestPush() {
	artifact_name := "System.Hunt.Participation"

	payload := []*ordereddict.Dict{
		ordereddict.NewDict().Set("foo", 1),
		ordereddict.NewDict().Set("foo", 2)}

	ctx := context.Background()
	output, cancel := self.manager.Watch(ctx, artifact_name)
	defer cancel()

	log_path := path_specs.NewUnsafeFilestorePath("log_path")
	err := self.manager.PushEventRows(
		MockPathManager{log_path, artifact_name},
		payload)

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
	assert.Contains(self.T(), self.FilestoreGet(log_path), "foo")
}

func NewQueueManagerTestSuite(
	config_obj *config_proto.Config,
	manager api.QueueManager,
	file_store api.FileStore) *QueueManagerTestSuite {
	return &QueueManagerTestSuite{
		config_obj: config_obj,
		manager:    manager,
		file_store: file_store,
	}
}

type MockPathManager struct {
	Path         api.FSPathSpec
	ArtifactName string
}

func (self MockPathManager) GetPathForWriting() (api.FSPathSpec, error) {
	return self.Path, nil
}

func (self MockPathManager) GetQueueName() string {
	return self.ArtifactName
}

func (self MockPathManager) GetAvailableFiles(
	ctx context.Context) []*api.ResultSetFileProperties {
	return nil
}
