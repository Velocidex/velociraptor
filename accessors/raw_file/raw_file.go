/* This accessor is used for reading raw devices.

On Windows, raw files need to be read in aligned page size. This
accessor ensures reads are buffered into page size buffers to make it
safe for VQL to read the device in arbitrary alignment.

We do not support directory operations on raw devices.

*/

package raw_file

import (
	"errors"
	"fmt"
	"os"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/files"
	"www.velocidex.com/golang/vfilter"
)

type RawFileSystemAccessor struct {
	scope vfilter.Scope
}

func (self RawFileSystemAccessor) ParsePath(path string) (*accessors.OSPath, error) {
	return accessors.NewRawFilePath(path)
}

func (self RawFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	result := &RawFileSystemAccessor{
		scope: scope,
	}
	return result, nil
}

func (self RawFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name: "raw_file",
		Description: `Access a device using aligned reads.

On Windows device reads must be aligned to page size.

This accessor ensures all reads are aligned.

The accessor may be used on other files but:

1. Reads will be aligned to page size (4096 bytes)
2. The last page will be zero padded past the end of file.
`,
		Permissions: []acls.ACL_PERMISSION{acls.FILESYSTEM_READ},
	}
}

func (self RawFileSystemAccessor) ReadDir(
	path string) ([]accessors.FileInfo, error) {
	return nil, errors.New("Not Implemented")
}

func (self RawFileSystemAccessor) ReadDirWithOSPath(
	path *accessors.OSPath) ([]accessors.FileInfo, error) {
	return nil, errors.New("Not Implemented")
}

func (self RawFileSystemAccessor) OpenWithOSPath(
	filename *accessors.OSPath) (accessors.ReadSeekCloser, error) {
	return self.Open(filename.Path())
}

func (self RawFileSystemAccessor) Open(filename string) (accessors.ReadSeekCloser, error) {
	// Treat the path as a raw OS path.
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("While opening %v: %v", filename, err)
	}

	files.Add(filename)

	reader, err := ntfs.NewPagedReader(file, 0x1000, 10000)
	if err != nil {
		return nil, err
	}

	res := utils.NewReadSeekReaderAdapter(reader, func() {
		files.Remove(filename)
	})

	// Try to figure out the size - not necessary but in case we can
	// we can limit readers to this size.
	stat, err := os.Lstat(filename)
	if err == nil {
		res.SetSize(stat.Size())
	}

	return res, nil
}

func (self RawFileSystemAccessor) Lstat(path string) (accessors.FileInfo, error) {
	full_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	stat, err := os.Lstat(path)
	if err != nil {
		// On Windows it is not always possible to stat a device. In
		// that case we need to return a fake object so it is not an
		// error.
		stat = &accessors.VirtualFileInfo{
			Path:  full_path,
			Size_: 1<<63 - 1,
		}
	}

	return file.NewOSFileInfo(stat, full_path), err
}

func (self RawFileSystemAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (accessors.FileInfo, error) {

	path := full_path.String()
	stat, err := os.Lstat(path)
	if err != nil {
		// On Windows it is not always possible to stat a device. In
		// that case we need to return a fake object so it is not an
		// error.
		stat = &accessors.VirtualFileInfo{
			Path:  full_path,
			Size_: 1<<63 - 1,
		}
	}

	return file.NewOSFileInfo(stat, full_path), err
}

func init() {
	accessors.Register(&RawFileSystemAccessor{})
}
