package ntfs

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

func TestNTFSFilesystemAccessor(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, vql_subsystem.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	abs_path, _ := filepath.Abs("../../artifacts/testdata/files/test.ntfs.dd")
	fs_accessor := NewNTFSFileSystemAccessor(scope, abs_path, "file")

	root_path := accessors.NewWindowsOSPath("")

	globber := glob.NewGlobber()
	globber.Add(accessors.NewWindowsOSPath("/*"))

	hits := []string{}
	for hit := range globber.ExpandWithContext(
		context.Background(), config_obj, root_path, fs_accessor) {
		hits = append(hits, hit.OSPath().PathSpec().Path)
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
	root_fs_accessor := accessors.NewVirtualFilesystemAccessor()
	root_fs_accessor.SetVirtualFileInfo(&accessors.VirtualFileInfo{
		Path:   accessors.NewWindowsOSPath("\\\\.\\C:"),
		IsDir_: true,
	})

	root_fs_accessor.SetVirtualFileInfo(&accessors.VirtualFileInfo{
		Path:   accessors.NewWindowsOSPath("\\\\.\\D:"),
		IsDir_: true,
	})

	// Overlay a MountFileSystemAccessor over the
	// VirtualFilesystemAccessor. We will use Windows path
	// convensions so it looks like a real windows system.
	mount_fs := accessors.NewMountFileSystemAccessor(
		accessors.NewWindowsOSPath(""), root_fs_accessor)

	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, vql_subsystem.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	abs_path, _ := filepath.Abs("../../artifacts/testdata/files/test.ntfs.dd")
	c_fs_accessor := NewNTFSFileSystemAccessor(scope, abs_path, "file")
	d_fs_accessor := NewNTFSFileSystemAccessor(scope, abs_path, "file")

	// Mount the ntfs accessors on the C and D devices
	mount_fs.AddMapping(
		accessors.NewWindowsOSPath(""), // Mount at the root of the filesystem
		accessors.NewWindowsOSPath("\\\\.\\C:"),
		c_fs_accessor)

	mount_fs.AddMapping(
		accessors.NewWindowsOSPath(""),
		accessors.NewWindowsOSPath("\\\\.\\D:"),
		d_fs_accessor)

	// Start globbing from the top level.
	root_path := accessors.NewWindowsOSPath("")

	// Find all $MFT files
	globber := glob.NewGlobber()
	globber.Add(accessors.NewWindowsOSPath("/*/$MFT"))

	hits := []string{}
	for hit := range globber.ExpandWithContext(
		context.Background(), config_obj, root_path, mount_fs) {
		hits = append(hits, hit.FullPath())
	}

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
