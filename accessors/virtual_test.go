package accessors

import (
	"io/ioutil"
	"os"
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestVirtualFilesystemAccessor(t *testing.T) {
	fs_accessor := NewVirtualFilesystemAccessor()
	fs_accessor.SetVirtualDirectory(
		NewLinuxOSPath("/foo/bar/baz"), &VirtualFileInfo{
			IsDir_: true,
		})
	fs_accessor.SetVirtualDirectory(
		NewLinuxOSPath("/foo/bar2/x"), &VirtualFileInfo{
			RawData: []byte("Hello"),
		})
	fs_accessor.SetVirtualDirectory(
		NewLinuxOSPath("/foo/bar2/y"), &VirtualFileInfo{
			RawData: []byte("Goodbye"),
		})

	ls := func(path string) []string {
		children, err := fs_accessor.ReadDir(path)
		assert.NoError(t, err)

		results := []string{}
		for _, c := range children {
			results = append(results, c.FullPath())
		}
		return results
	}
	assert.Equal(t, []string{"/foo/bar", "/foo/bar2"}, ls("/foo"))
	assert.Equal(t, []string{"/foo/bar/baz"}, ls("/foo/bar"))
	assert.Equal(t, []string{"/foo/bar2/x", "/foo/bar2/y"}, ls("/foo/bar2"))

	// Check the file contents
	cat := func(path string) string {
		fd, err := fs_accessor.Open(path)
		assert.NoError(t, err)

		data, err := ioutil.ReadAll(fd)
		assert.NoError(t, err)

		return string(data)
	}

	assert.Equal(t, "Hello", cat("/foo/bar2/x"))
	assert.Equal(t, "Goodbye", cat("/foo/bar2/y"))

	// Check stats
	stat := func(path string) FileInfo {
		stat, err := fs_accessor.Lstat(path)
		assert.NoError(t, err)
		return stat
	}

	// Interpolated directory
	assert.Equal(t, true, stat("/foo").IsDir())
	assert.Equal(t, false, stat("/foo/bar2/y").IsDir())
	assert.Equal(t, true, stat("/foo/bar/baz").IsDir())

	// Missing files
	_, err := fs_accessor.ReadDir("/nosuchfile")
	assert.True(t, os.IsNotExist(err))
}
