//go:build linux
// +build linux

package file

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type AccessorLinuxTestSuite struct {
	suite.Suite
	tmpdir string
}

func (self *AccessorLinuxTestSuite) TestLinuxSymlinks() {
	tmpdir, err := tempfile.TempDir("accessor_test")
	assert.NoError(self.T(), err)

	// Create two symlinks.
	// tmp/second_bin/ -> tmp/zbin
	// tmp/zbin -> /bin/

	err = os.Symlink("/bin", filepath.Join(tmpdir, "zbin"))
	assert.NoError(self.T(), err)

	err = os.Symlink(filepath.Join(tmpdir, "zbin"),
		filepath.Join(tmpdir, "second_bin"))
	assert.NoError(self.T(), err)

	// Create a symlink cycle:
	// tmp/subdir is a directory
	// tmp/sym1 -> tmp/subdir
	// tmp/subdir/sym2 -> tmp

	dirname := filepath.Join(tmpdir, "subdir")
	err = os.Mkdir(dirname, 0777)
	assert.NoError(self.T(), err)

	err = os.Mkdir(filepath.Join(dirname, "ls"), 0777)
	assert.NoError(self.T(), err)

	err = os.Symlink(dirname, filepath.Join(tmpdir, "sym1"))
	assert.NoError(self.T(), err)

	err = os.Symlink(tmpdir, filepath.Join(dirname, "sym2"))
	assert.NoError(self.T(), err)

	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	glob_path, _ := accessors.NewLinuxOSPath("/**/ls")
	tmp_path, _ := accessors.NewLinuxOSPath(tmpdir)

	options := glob.GlobOptions{
		DoNotFollowSymlinks: false,
	}
	globber := glob.NewGlobber().WithOptions(options)
	defer globber.Close()

	globber.Add(glob_path)

	accessor, err := accessors.GetAccessor("file", scope)
	assert.NoError(self.T(), err)

	config_obj := config.GetDefaultConfig()
	hits := []string{}
	for hit := range globber.ExpandWithContext(
		context.Background(), scope,
		config_obj, tmp_path, accessor) {
		hits = append(hits, hit.OSPath().TrimComponents(
			tmp_path.Components...).String())
	}

	sort.Strings(hits)

	goldie.Assert(self.T(), "TestLinuxSymlinks", json.MustMarshalIndent(hits))
}

// Test Linux specific File accessor.
func TestFileLinux(t *testing.T) {
	suite.Run(t, &AccessorLinuxTestSuite{})
}
