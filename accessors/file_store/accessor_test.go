package file_store

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var (
	files = []string{
		"fs:/Windows/System32/notepad.exe",
		"fs:/Windows/System32/NotePad2.exe",
	}
)

type FSAccessorTest struct {
	test_utils.TestSuite

	config_obj *config_proto.Config
	file_store *directory.DirectoryFileStore
}

func (self *FSAccessorTest) TestCaseInsensitive() {
	accessor := NewFileStoreFileSystemAccessor(self.ConfigObj)

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

		// Now open the same file with the wrong casing.
		fd, err = accessor.Open(strings.ToLower(f))
		assert.NoError(self.T(), err)

		n, err = fd.Read(buf)
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), n, 5)
		assert.Equal(self.T(), string(buf[:n]), "hello")
	}

}

func TestFileStoreAccessor(t *testing.T) {
	suite.Run(t, &FSAccessorTest{})
}
