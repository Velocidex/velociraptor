package ntfs

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

func TestNTFSFilesystemAccessor(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	abs_path, _ := filepath.Abs("../../artifacts/testdata/files/test.ntfs.dd")
	root_path := accessors.MustNewWindowsOSPath("")

	fs_accessor := NewNTFSFileSystemAccessor(
		scope, root_path, accessors.MustNewGenericOSPath(abs_path), "file")

	globber := glob.NewGlobber()
	defer globber.Close()

	globber.Add(accessors.MustNewWindowsOSPath("/*"))

	hits := []string{}
	for hit := range globber.ExpandWithContext(
		context.Background(), scope, config_obj, root_path, fs_accessor) {
		hits = append(hits, hit.OSPath().String())
	}

	goldie.Assert(t, "TestNTFSFilesystemAccessor", json.MustMarshalIndent(hits))

	buffer := make([]byte, 40)
	fd, err := fs_accessor.Open("/ones.bin")
	assert.NoError(t, err)

	_, err = fd.Read(buffer)
	assert.NoError(t, err)

	assert.Equal(t, "ONESONESONESONESONESONESONESONESONESONES", string(buffer))
}

// Here we build a remapping of the same ntfs image between two mount points.
func TestNTFSFilesystemAccessorRemapping(t *testing.T) {
	config_obj := config.GetDefaultConfig()

	// Create the two mount point directories in the VirtualFilesystemAccessor
	root_path := accessors.MustNewWindowsOSPath("")
	root_fs_accessor := accessors.NewVirtualFilesystemAccessor(root_path)
	root_fs_accessor.SetVirtualFileInfo(&accessors.VirtualFileInfo{
		Path:   accessors.MustNewWindowsOSPath("\\\\.\\C:"),
		IsDir_: true,
	})

	root_fs_accessor.SetVirtualFileInfo(&accessors.VirtualFileInfo{
		Path:   accessors.MustNewWindowsOSPath("\\\\.\\D:"),
		IsDir_: true,
	})

	// Overlay a MountFileSystemAccessor over the
	// VirtualFilesystemAccessor. We will use Windows path
	// convensions so it looks like a real windows system.
	mount_fs := accessors.NewMountFileSystemAccessor(
		accessors.MustNewWindowsOSPath(""), root_fs_accessor)

	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	abs_path, _ := filepath.Abs("../../artifacts/testdata/files/test.ntfs.dd")
	c_fs_accessor := NewNTFSFileSystemAccessor(
		scope, root_path, accessors.MustNewGenericOSPath(abs_path), "file")
	d_fs_accessor := NewNTFSFileSystemAccessor(
		scope, root_path, accessors.MustNewGenericOSPath(abs_path), "file")

	// Mount the ntfs accessors on the C and D devices
	mount_fs.AddMapping(
		accessors.MustNewWindowsOSPath(""), // Mount at the root of the filesystem
		accessors.MustNewWindowsOSPath("\\\\.\\C:"),
		c_fs_accessor)

	mount_fs.AddMapping(
		accessors.MustNewWindowsOSPath(""),
		accessors.MustNewWindowsOSPath("\\\\.\\D:"),
		d_fs_accessor)

	// Start globbing from the top level.
	// Find all $MFT files
	globber := glob.NewGlobber()
	defer globber.Close()

	globber.Add(accessors.MustNewWindowsOSPath("/*/$MFT"))

	hits := []string{}
	for hit := range globber.ExpandWithContext(
		context.Background(), scope, config_obj, accessors.MustNewWindowsOSPath(""),
		mount_fs) {
		hits = append(hits, hit.FullPath())
	}

	sort.Strings(hits)

	goldie.Assert(t, "TestNTFSFilesystemAccessorRemapping",
		json.MustMarshalIndent(hits))

	// Now open a file for reading.
	buffer := make([]byte, 40)
	fd, err := mount_fs.Open("\\\\.\\C:\\ones.bin")
	assert.NoError(t, err)

	_, err = fd.Read(buffer)
	assert.NoError(t, err)

	assert.Equal(t, "ONESONESONESONESONESONESONESONESONESONES", string(buffer))
}
