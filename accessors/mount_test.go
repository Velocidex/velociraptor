package accessors

import (
	"io/ioutil"
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestMountFilesystemAccessor(t *testing.T) {
	// The root filesystem contains some directories where the other
	// filesystems are mounted.
	root_path := MustNewLinuxOSPath("")

	root_fs_accessor := NewVirtualFilesystemAccessor(root_path)
	root_fs_accessor.SetVirtualDirectory(
		MustNewLinuxOSPath("/usr"),
		&VirtualFileInfo{
			IsDir_: true,
		})

	root_fs_accessor.SetVirtualDirectory(
		MustNewLinuxOSPath("/home"), &VirtualFileInfo{
			IsDir_: true,
		})
	root_fs_accessor.SetVirtualDirectory(
		MustNewLinuxOSPath("/lib/foo"), &VirtualFileInfo{
			RawData: []byte("lib foo file"),
		})

	// Child filesystem contains some files.
	bin_fs_accessor := NewVirtualFilesystemAccessor(root_path)
	bin_fs_accessor.SetVirtualDirectory(
		MustNewLinuxOSPath("/bin/ls"), &VirtualFileInfo{
			RawData: []byte("bin ls file"),
		})

	// This will contain a deeper mount again.
	bin_fs_accessor.SetVirtualDirectory(
		MustNewLinuxOSPath("/bin/deep"), &VirtualFileInfo{
			IsDir_: true,
		})

	bin_fs_accessor.SetVirtualDirectory(
		MustNewLinuxOSPath("/bin/foo/bar"), &VirtualFileInfo{
			RawData: []byte("bar file"),
		})

	bin_fs_accessor.SetVirtualDirectory(
		MustNewLinuxOSPath("/bin/foo/baz_dir"), &VirtualFileInfo{
			IsDir_: true,
		})

	bin_fs_accessor.SetVirtualDirectory(
		MustNewLinuxOSPath("/bin/foo/baz_dir/baz"), &VirtualFileInfo{
			RawData: []byte("baz file"),
		})

	// Another filesystem will be mounted deeper again
	deep_fs := NewVirtualFilesystemAccessor(root_path)
	deep_fs.SetVirtualDirectory(
		MustNewLinuxOSPath("/Users/mic/test.txt"), &VirtualFileInfo{
			RawData: []byte("text"),
		})

	// Create a mount filesystem to organize the different
	// filesystems. Use Linux path convensions.
	mount_fs := NewMountFileSystemAccessor(
		MustNewLinuxOSPath(""), root_fs_accessor)

	// This means the root of the bin_fs_accessor is mounted at /usr
	mount_fs.AddMapping(
		MustNewLinuxOSPath("/"),
		MustNewLinuxOSPath("/usr"), bin_fs_accessor)

	// It is also possible to mount into a directory inside another
	// filesystem. This is similar to NTFS hard links or Linux "bind"
	// mounts. The following means that the tree under /home is taken
	// from /bin/foo/ on the bin_fs_accessor
	mount_fs.AddMapping(
		MustNewLinuxOSPath("/bin/foo"),
		MustNewLinuxOSPath("/home"), bin_fs_accessor)

	// Mount deep_fs inside the bin_fs_accessor mount point
	mount_fs.AddMapping(
		MustNewLinuxOSPath("/"),
		MustNewLinuxOSPath("/usr/bin/deep"), deep_fs)

	ls := func(path string) []string {
		children, err := mount_fs.ReadDir(path)
		assert.NoError(t, err)

		results := []string{}
		for _, c := range children {
			results = append(results, c.FullPath())
		}
		//fmt.Printf("ls %v -> %v\n", path, results)
		return results
	}

	// Listing the root filesystem
	assert.Equal(t, []string{"/usr", "/home", "/lib"}, ls("/"))
	assert.Equal(t,
		[]string{"/usr/bin/ls", "/usr/bin/deep", "/usr/bin/foo"},
		ls("/usr/bin"))

	// /usr/bin/deep/Users/mic/ is mounted twice:
	// 1. /usr/bin is mounted to bin_fs_accessor
	// 2. /usr/bin/deep is mounted to deep_fs
	assert.Equal(t,
		[]string{"/usr/bin/deep/Users/mic/test.txt"},
		ls("/usr/bin/deep/Users/mic/"))

	assert.Equal(t,
		[]string{"/usr/bin/deep/Users/mic"},
		ls("/usr/bin/deep/Users/"))

	// Check bind mount - /home directory comes from /bin/foo
	assert.Equal(t,
		[]string{"/home/bar", "/home/baz_dir"},
		ls("/home/"))

	assert.Equal(t,
		[]string{"/home/baz_dir/baz"},
		ls("/home/baz_dir"))

	// Check the file contents
	cat := func(path string) string {
		fd, err := mount_fs.Open(path)
		assert.NoError(t, err)

		data, err := ioutil.ReadAll(fd)
		assert.NoError(t, err)

		return string(data)
	}

	assert.Equal(t, "text", cat("/usr/bin/deep/Users/mic/test.txt"))
}
