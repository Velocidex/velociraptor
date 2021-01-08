// Implements an automatic fallback to NTFS accessor when
// OSFileSystemAccessor does not work.

package filesystems

import (
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

type AutoFilesystemAccessor struct {
	ntfs_delegate glob.FileSystemAccessor
	file_delegate glob.FileSystemAccessor
}

func (self AutoFilesystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	ntfs_base, err := NTFSFileSystemAccessor{}.New(scope)
	if err != nil {
		return nil, err
	}

	os_base, err := OSFileSystemAccessor{}.New(scope)
	if err != nil {
		return nil, err
	}

	return &AutoFilesystemAccessor{
		ntfs_delegate: ntfs_base,
		file_delegate: os_base,
	}, nil
}

func (self *AutoFilesystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	result, err := self.file_delegate.ReadDir(path)
	if err != nil {
		return self.ntfs_delegate.ReadDir(path)
	}
	return result, err
}

func (self *AutoFilesystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	result, err := self.file_delegate.Open(path)
	if err != nil {
		return self.ntfs_delegate.Open(path)
	}
	return result, err
}

func (self *AutoFilesystemAccessor) Lstat(path string) (glob.FileInfo, error) {
	result, err := self.file_delegate.Lstat(path)
	if err != nil {
		return self.ntfs_delegate.Lstat(path)
	}
	return result, err
}

func (self *AutoFilesystemAccessor) GetRoot(path string) (
	device string, subpath string, err error) {
	device, subpath, err = self.file_delegate.GetRoot(path)
	if err != nil {
		return self.ntfs_delegate.GetRoot(path)
	}
	return
}

func (self AutoFilesystemAccessor) PathJoin(x, y string) string {
	return self.file_delegate.PathJoin(x, y)
}

func (self *AutoFilesystemAccessor) PathSplit(path string) []string {
	return self.file_delegate.PathSplit(path)
}

func init() {
	glob.Register("file", &AutoFilesystemAccessor{})
	glob.Register("auto", &AutoFilesystemAccessor{})
}
