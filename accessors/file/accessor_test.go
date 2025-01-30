package file_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
)

type AccessorWindowsTestSuite struct {
	suite.Suite
	tmpdir string
}

func (self *AccessorWindowsTestSuite) SetupTest() {
	tmpdir, err := tempfile.TempDir("accessor_test")
	assert.NoError(self.T(), err)

	self.tmpdir = strings.ReplaceAll(tmpdir, "\\", "/")
}

func (self *AccessorWindowsTestSuite) TearDownTest() {
	os.RemoveAll(self.tmpdir) // clean up
}

func (self *AccessorWindowsTestSuite) TestACL() {
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	accessor, err := accessors.GetAccessor("file", scope)
	// Permission denied!
	assert.Error(self.T(), err)

	// Try again with more premissions.
	scope = vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	accessor, err = accessors.GetAccessor("file", scope)
	assert.NoError(self.T(), err)

	_, err = accessor.ReadDir("/")
	assert.NoError(self.T(), err)
}

// This test will just pass on Windows in any case but will fail on
// linux if the file_nocase is broken.
func (self *AccessorWindowsTestSuite) TestNoCase() {
	dirname := filepath.Join(self.tmpdir, "some/test/directory/with/parent")
	err := os.MkdirAll(dirname, 0777)
	assert.NoError(self.T(), err)

	file_path := filepath.Join(dirname, "1.txt")
	fd, err := os.OpenFile(file_path,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
	assert.NoError(self.T(), err)
	fd.Write([]byte("Hello world"))
	fd.Close()

	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))
	accessor, err := accessors.GetAccessor("file_nocase", scope)
	assert.NoError(self.T(), err)

	// Open the file with many different casing. Mismatched casing at
	// deeper directories should also work.
	for _, filename := range []string{
		"%s/some/test/directory/with/parent/1.txt",
		"%s/some/test/directory/with/parent/1.TxT",
		"%s/some/test/directory/With/parent/1.txt",
		"%s/some/test/DiRectory/With/parent/1.txt",
		"%s/Some/test/DiRectory/With/parent/1.txt",
	} {
		interpolated_path := strings.ReplaceAll(
			fmt.Sprintf(filename, self.tmpdir), "\\", "\\\\")
		reader, err := accessor.Open(interpolated_path)
		assert.NoError(self.T(), err)
		defer fd.Close()

		data := make([]byte, 100)
		n, err := reader.Read(data)
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), string(data[:n]), "Hello world")
	}
}

// This looks like
// tmpdir/subdir/1.txt
// tmpdir/subdir/link1 -> tmpdir/subdir/1.txt
// tmpdir/subdir/parent_link -> tmpdir/subdir
// tmpdir/subdir/dir_link -> tmpdir/subdir/parent_link
func (self *AccessorWindowsTestSuite) TestSymlinks() {
	// This test only works on Linux and MacOS
	if runtime.GOOS == "windows" {
		return
	}

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

	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))
	accessor, err := accessors.GetAccessor("file", scope)
	assert.NoError(self.T(), err)

	// Open through the link.
	for _, filename := range []string{
		"%s/subdir/1.txt",
		"%s/subdir/parent_link/subdir/1.txt",
		"%s/subdir/dir_link/subdir/1.txt",

		// Accept a pathspec as well.
		`{"Path":"%s/subdir/1.txt"}`,
	} {
		interpolated_path := strings.ReplaceAll(
			fmt.Sprintf(filename, self.tmpdir), "\\", "\\\\")
		reader, err := accessor.Open(interpolated_path)
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
	globber := glob.NewGlobber()
	defer globber.Close()

	glob_path, _ := accessors.NewGenericOSPath("**/*.txt")
	globber.Add(glob_path)

	hits := []string{}
	tmp_path, _ := accessors.NewGenericOSPath(self.tmpdir)
	for hit := range globber.ExpandWithContext(
		context.Background(), scope,
		config_obj, tmp_path, accessor) {
		hits = append(hits, strings.ReplaceAll(
			strings.TrimPrefix(hit.FullPath(), self.tmpdir), "\\", "/"))
	}

	assert.Equal(self.T(),
		[]string{"/subdir/1.txt", "/subdir/dir_link/subdir/1.txt"},
		hits)

}

// Test both the Windows and Linux File accessor.
func TestWindowsLinux(t *testing.T) {
	suite.Run(t, &AccessorWindowsTestSuite{})
}
