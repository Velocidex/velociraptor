// +build windows

// Implements an automatic fallback to NTFS accessor when
// OSFileSystemAccessor does not work.

package file

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/vfilter"
)

type AutoFilesystemAccessor struct {
	ntfs_delegate accessors.FileSystemAccessor
	file_delegate accessors.FileSystemAccessor
}

func (self AutoFilesystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewWindowsOSPath(path)
}

func (self AutoFilesystemAccessor) New(scope vfilter.Scope) (accessors.FileSystemAccessor, error) {
	ntfs_base, err := accessors.GetAccessor("ntfs", scope)
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

func (self *AutoFilesystemAccessor) ReadDirWithOSPath(
	path *accessors.OSPath) ([]accessors.FileInfo, error) {
	result, err := self.file_delegate.ReadDirWithOSPath(path)
	if err != nil {
		ntfs_path := accessors.WindowsNTFSPathFromOSPath(path)
		return self.ntfs_delegate.ReadDirWithOSPath(ntfs_path)
	}
	return result, err
}

func (self *AutoFilesystemAccessor) ReadDir(path string) ([]accessors.FileInfo, error) {
	result, err := self.file_delegate.ReadDir(path)
	if err != nil {
		return self.ntfs_delegate.ReadDir(path)
	}
	return result, err
}

func (self *AutoFilesystemAccessor) Open(path string) (accessors.ReadSeekCloser, error) {
	result, err := self.file_delegate.Open(path)
	if err != nil {
		result, err1 := self.ntfs_delegate.Open(path)
		if err1 != nil {
			return nil, fmt.Errorf(
				"%v, unable to fall back to ntfs parsing: %w", err, err1)
		}
		return result, err1
	}
	return result, err
}

func (self *AutoFilesystemAccessor) OpenWithOSPath(path *accessors.OSPath) (accessors.ReadSeekCloser, error) {
	result, err := self.file_delegate.OpenWithOSPath(path)
	if err != nil {
		ntfs_path := accessors.WindowsNTFSPathFromOSPath(path)
		result, err1 := self.ntfs_delegate.OpenWithOSPath(ntfs_path)
		if err1 != nil {
			return nil, fmt.Errorf(
				"%v, unable to fall back to ntfs parsing: %w", err, err1)
		}
		return result, err1
	}
	return result, err
}

func (self *AutoFilesystemAccessor) Lstat(path string) (accessors.FileInfo, error) {
	result, err := self.file_delegate.Lstat(path)
	if err != nil {
		return self.ntfs_delegate.Lstat(path)
	}
	return result, err
}

func (self *AutoFilesystemAccessor) LstatWithOSPath(
	path *accessors.OSPath) (accessors.FileInfo, error) {
	result, err := self.file_delegate.LstatWithOSPath(path)
	if err != nil {
		ntfs_path := accessors.WindowsNTFSPathFromOSPath(path)
		return self.ntfs_delegate.LstatWithOSPath(ntfs_path)
	}
	return result, err
}

func init() {
	accessors.Register("auto", &AutoFilesystemAccessor{},
		`Automatically access the filesystem using the best method.

On Windows, we fallback to ntfs accessor if the file is not readable or locked.
`)
}
