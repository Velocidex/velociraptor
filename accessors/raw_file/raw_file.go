/* This accessor is used for reading raw devices.

On Windows, raw files need to be read in aligned page size. This
accessor ensures reads are buffered into page size buffers to make it
safe for VQL to read the device in arbitrary alignment.

We do not support directory operations on raw devices.

*/

package raw_file

import (
	"errors"
	"os"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type RawFileSystemAccessor struct{}

func (self RawFileSystemAccessor) ParsePath(path string) *accessors.OSPath {
	return accessors.NewPathspecOSPath(path)
}

func (self RawFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	// Check we have permission to open files.
	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_READ)
	if err != nil {
		return nil, err
	}

	result := &RawFileSystemAccessor{}
	return result, nil
}

func (self RawFileSystemAccessor) ReadDir(path string) ([]accessors.FileInfo, error) {
	return nil, errors.New("Not Implemented")
}

func (self RawFileSystemAccessor) Open(filename string) (accessors.ReadSeekCloser, error) {
	// Treat the path as a raw OS path.
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	reader, err := ntfs.NewPagedReader(file, 0x1000, 10000)
	if err != nil {
		return nil, err
	}

	return utils.NewReadSeekReaderAdapter(reader), nil
}

func (self RawFileSystemAccessor) Lstat(path string) (accessors.FileInfo, error) {
	return &file.OSFileInfo{}, nil
}

func init() {
	accessors.Register("raw_file", &RawFileSystemAccessor{},
		`Access the filesystem using the OS API.`)
}
