// +build linux

package glob

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/vfilter"
)

type AccessorLinuxTestSuite struct {
	suite.Suite
	tmpdir string
}

func (self *AccessorLinuxTestSuite) SetupTest() {
	tmpdir, err := ioutil.TempDir("", "accessor_test")
	assert.NoError(self.T(), err)

	self.tmpdir = tmpdir
}

func (self *AccessorLinuxTestSuite) TearDownTest() {
	os.RemoveAll(self.tmpdir) // clean up
}

// This looks like
// tmpdir/subdir/1.txt
// tmpdir/subdir/link1 -> tmpdir/subdir/1.txt
// tmpdir/subdir/parent_link -> tmpdir/subdir
// tmpdir/subdir/dir_link -> tmpdir/subdir/parent_link
func (self *AccessorLinuxTestSuite) TestSymlinks() {
	dirname := filepath.Join(self.tmpdir, "subdir")
	err := os.Mkdir(dirname, 0777)
	assert.NoError(self.T(), err)

	file_path := filepath.Join(dirname, "1.txt")
	fd, err := os.OpenFile(file_path,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
	assert.NoError(self.T(), err)
	fd.Write([]byte("Hello world"))
	fd.Close()

	// Create a symlink to the file
	link_path := filepath.Join(dirname, "link1")
	err = os.Symlink(file_path, link_path)
	assert.NoError(self.T(), err)

	// Create a recursive symlink to parent directory
	parent_link_path := filepath.Join(dirname, "parent_link")
	err = os.Symlink(self.tmpdir, parent_link_path)
	assert.NoError(self.T(), err)

	// Create a recursive symlink to parent directory
	dir_link_path := filepath.Join(dirname, "dir_link")
	err = os.Symlink(parent_link_path, dir_link_path)
	assert.NoError(self.T(), err)

	scope := vfilter.NewScope()
	accessor, err := GetAccessor("file", scope)
	assert.NoError(self.T(), err)

	// Open through the link.
	for _, filename := range []string{
		"subdir/1.txt",
		"subdir/parent_link/subdir/1.txt",
		"subdir/dir_link/subdir/1.txt"} {
		reader, err := accessor.Open(filepath.Join(self.tmpdir, filename))
		assert.NoError(self.T(), err)
		defer fd.Close()

		data := make([]byte, 100)
		n, err := reader.Read(data)
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), string(data[:n]), "Hello world")
	}

	config_obj := config.GetDefaultConfig()

	// Now glob through the files - this should not lock up since
	// the cycle should be detected.
	globber := NewGlobber()
	globber.Add("**/*.txt", accessor.PathSplit)

	hits := []string{}
	for hit := range globber.ExpandWithContext(context.Background(),
		config_obj, self.tmpdir, accessor) {
		hits = append(hits, strings.TrimPrefix(hit.FullPath(), self.tmpdir))
	}

	assert.Equal(self.T(),
		[]string{"/subdir/1.txt"},
		hits)

}

func TestAccessorLinux(t *testing.T) {
	suite.Run(t, &AccessorLinuxTestSuite{})
}
