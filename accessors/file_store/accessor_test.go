package file_store_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type testCase struct {
	checked string
	err     error
}

var (
	files = []string{
		"fs:/Windows/System32/notepad.exe",
		"fs:/Windows/System32/NotePad2.exe",

		// Filestores allow data to be stored in a "directory"
		"fs:/Windows/System32",
	}

	checked_files = []testCase{
		{checked: "fs:/WinDowS/SySteM32/NotePad.exe"},
		{checked: "fs:/Windows/System32/NotePad2.exe"},

		// Filestores allow data to be stored in a "directory"
		{checked: "fs:/windows/system32"},

		{checked: "fs:/Windows/System32/DoesNotExist.exe",
			err: utils.NotFoundError},
	}
)

type FSAccessorTest struct {
	test_utils.TestSuite

	config_obj *config_proto.Config
}

func (self *FSAccessorTest) TestCaseInsensitive() {
	accessor := file_store_accessor.NewFileStoreFileSystemAccessor(self.ConfigObj)

	file_store_factory := file_store.GetFileStore(self.ConfigObj)

	buf := make([]byte, 100)

	// Create some files with data
	for _, f := range files {
		pathspec, err := accessor.ParsePath(f)
		assert.NoError(self.T(), err)

		fullpath := path_specs.FromGenericComponentList(pathspec.Components)
		w, err := file_store_factory.WriteFile(fullpath)
		assert.NoError(self.T(), err)
		w.Write([]byte("hello"))
		w.Close()

		// Use the accessor to open a file directly.
		fd, err := accessor.Open(f)
		assert.NoError(self.T(), err)

		n, err := fd.Read(buf)
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), n, 5)
		assert.Equal(self.T(), string(buf[:n]), "hello")
	}

	for _, testcase := range checked_files {
		// Now open the same file with the wrong casing.
		fd, err := accessor.Open(testcase.checked)
		if testcase.err != nil {
			assert.True(self.T(), errors.Is(err, testcase.err))
			continue
		}
		assert.NoError(self.T(), err)

		n, err := fd.Read(buf)
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), n, 5)
		assert.Equal(self.T(), string(buf[:n]), "hello")
	}
}

func (self *FSAccessorTest) TestSparseFiles() {
	filename := path_specs.FromGenericComponentList([]string{"Test.txt"}).
		SetType(api.PATH_TYPE_FILESTORE_ANY)
	filename_idx := filename.SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX)

	file_store_factory := file_store.GetFileStore(self.ConfigObj)

	w, err := file_store_factory.WriteFile(filename)
	assert.NoError(self.T(), err)
	w.Write([]byte("HelloWorld"))
	w.Close()

	// Only 10 bytes are written to the filestore.
	stat_file, err := file_store_factory.StatFile(filename)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), stat_file.Size(), int64(10))

	w, err = file_store_factory.WriteFile(filename_idx)
	assert.NoError(self.T(), err)

	// Original offset refers to the offset in the remote sparse file.
	// file offset refers to the offset within the filestore file
	w.Write([]byte(`
{
 "ranges": [
  {
   "file_offset": 0,
   "original_offset": 0,
   "file_length": 5,
   "length": 5
  },
  {
   "file_offset": 5,
   "original_offset": 5,
   "length": 5,
   "file_length": 0
  },
  {
   "file_offset": 5,
   "original_offset": 10,
   "file_length": 5,
   "length": 5
  }
 ]
}`)) // This represents: Hello<.....>World with the gap being sparse.
	w.Close()

	accessor := file_store_accessor.NewSparseFileStoreFileSystemAccessor(self.ConfigObj)
	fd, err := accessor.Open(filename.Components()[0])
	assert.NoError(self.T(), err)

	buf := make([]byte, 100)
	n, err := fd.Read(buf)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), string(buf[:n]), "Hello\x00\x00\x00\x00\x00World")

	// Check that the file handle can report its ranges
	fd_ranges, ok := fd.(uploads.RangeReader)
	assert.True(self.T(), ok)

	// An Lstat() reports the sparse file size as 15 - including the sparse hole.
	stat, err := accessor.Lstat(filename.Components()[0])
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), stat.Size(), int64(15))

	goldie.Assert(self.T(), "TestSparseFiles",
		json.MustMarshalIndent(fd_ranges.Ranges()))
}

func TestFileStoreAccessor(t *testing.T) {
	suite.Run(t, &FSAccessorTest{})
}
