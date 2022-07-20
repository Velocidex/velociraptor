package ntfs

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestMFTFilesystemAccessor(t *testing.T) {
	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	abs_path, _ := filepath.Abs("../../artifacts/testdata/files/test.ntfs.dd")
	fs_accessor, err := MFTFileSystemAccessor{}.New(scope)
	assert.NoError(t, err)

	pathspec := accessors.MustNewPathspecOSPath(accessors.PathSpec{
		Path:             "38-128-0",
		DelegateAccessor: "file",
		DelegatePath:     abs_path,
	}.String())

	buffer := make([]byte, 40)
	fd, err := fs_accessor.OpenWithOSPath(pathspec)
	assert.NoError(t, err)

	_, err = fd.Read(buffer)
	assert.NoError(t, err)

	assert.Equal(t, "ONESONESONESONESONESONESONESONESONESONES", string(buffer))
}
