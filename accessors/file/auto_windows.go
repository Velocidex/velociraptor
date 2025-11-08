//go:build windows
// +build windows

// Implements an automatic fallback to NTFS accessor when
// OSFileSystemAccessor does not work.

package file

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/vfilter"
)

// Sometimes, we can open a file with the API ok, but we just can not
// read from it. This wrapper allows for switching to the ntfs parser
// after open, but if the file is not readable.  The following Python
// program creates a lock for testing. The following query will force
// a re-open with the ntfs accessor:
// SELECT read_file(filename='''C:\test.exe''',
//     accessor='auto', length=15)
// FROM scope()
/*
import win32file
import win32con
import win32security
import win32api
import pywintypes

highbits=0xffff0000 #high-order 32 bits of byte range to lock

file="C:\\test.exe"

secur_att = win32security.SECURITY_ATTRIBUTES()
secur_att.Initialize()

hfile=win32file.CreateFile(
    file,
    win32con.GENERIC_READ|win32con.GENERIC_WRITE,
    win32con.FILE_SHARE_READ|win32con.FILE_SHARE_WRITE,
    secur_att,
    win32con.OPEN_ALWAYS,
    win32con.FILE_ATTRIBUTE_NORMAL , 0 )

ov=pywintypes.OVERLAPPED()
win32file.LockFileEx(hfile,win32con.LOCKFILE_EXCLUSIVE_LOCK,10,highbits,ov)
win32api.Sleep(40000)
win32file.UnlockFileEx(hfile,0,highbits,ov)
hfile.Close()
*/
type FileReaderWrapper struct {
	readatter_mu sync.Mutex
	accessors.ReadSeekCloser

	mu sync.Mutex

	// If set, the reader is really an ntfs reader.
	switched_to_ntfs bool
	path             *accessors.OSPath

	owner *AutoFilesystemAccessor
}

func (self *FileReaderWrapper) ReadAt(buf []byte, offset int64) (int, error) {
	self.readatter_mu.Lock()
	defer self.readatter_mu.Unlock()

	_, err := self.Seek(offset, os.SEEK_SET)
	if err != nil {
		return 0, err
	}

	return self.Read(buf)
}

func (self *FileReaderWrapper) Read(buf []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	n, err := self.ReadSeekCloser.Read(buf)
	if err != nil &&
		shouldTryNTFS(self.path.Basename(), err) &&
		!self.switched_to_ntfs {

		// Reopen as an ntfs parsed file.
		self.path = accessors.WindowsNTFSPathFromOSPath(self.path)
		fd, err1 := self.owner.ntfs_delegate.OpenWithOSPath(self.path)
		if err1 != nil {
			return n, err
		}

		// Close the old reader and substitude a new one
		self.switched_to_ntfs = true
		current_offset, _ := self.ReadSeekCloser.Seek(0, os.SEEK_CUR)
		self.ReadSeekCloser.Close()

		fd.Seek(current_offset, os.SEEK_SET)
		self.ReadSeekCloser = fd

		// Try again with the new buffer.
		return fd.Read(buf)
	}
	return n, err
}

type AutoFilesystemAccessor struct {
	ntfs_delegate accessors.FileSystemAccessor
	file_delegate accessors.FileSystemAccessor
}

// On Windows filesystems are usually case insensitive.
func (self AutoFilesystemAccessor) GetCanonicalFilename(
	path *accessors.OSPath) string {
	return strings.ToLower(path.String())
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

func (self AutoFilesystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name: "auto",
		Description: `Automatically access the filesystem using the best method.

On Windows, we fallback to ntfs accessor if the file is not readable or locked.
`,
		Permissions: []acls.ACL_PERMISSION{acls.FILESYSTEM_READ},
	}
}

func (self *AutoFilesystemAccessor) GetUnderlyingAPIFilename(
	full_path *accessors.OSPath) (string, error) {
	return full_path.PathSpec().Path, nil
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
	pathspec, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}
	return self.OpenWithOSPath(pathspec)
}

func (self *AutoFilesystemAccessor) OpenWithOSPath(
	path *accessors.OSPath) (accessors.ReadSeekCloser, error) {
	result, err := self.file_delegate.OpenWithOSPath(path)
	if err != nil && shouldTryNTFS(path.Basename(), err) {
		ntfs_path := accessors.WindowsNTFSPathFromOSPath(path)
		result, err1 := self.ntfs_delegate.OpenWithOSPath(ntfs_path)
		if err1 != nil {
			return nil, fmt.Errorf(
				"%v, unable to fall back to ntfs parsing: %w", err, err1)
		}
		return result, err1
	}

	// Wrap the API handle in case we need to upgrade it in future
	return &FileReaderWrapper{
		ReadSeekCloser: result,
		path:           path,
		owner:          self,
	}, err
}

func shouldTryNTFS(path string, err error) bool {
	// Special NTFS files start with a $
	if strings.Contains(path, "\\$") || strings.HasPrefix(path, "$") {
		return true
	}

	// For permission denied we fallback to ntfs parsing.
	if errors.Is(err, os.ErrPermission) {
		return true
	}

	// These are regular errors - falling back to ntfs parsing will
	// not help much.
	if errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, os.ErrClosed) {
		return false
	}

	// If the file does not exist using the APIs then it is unlikely
	// that nts parsing will find it.
	if errors.Is(err, os.ErrNotExist) {
		return false
	}

	// This mostly occurs on directories.
	if strings.Contains(err.Error(), "Incorrect function") {
		return false
	}

	// Give ntfs parsing a shot - maybe it will work?
	return true
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
	if err != nil && shouldTryNTFS(path.Basename(), err) {
		ntfs_path := accessors.WindowsNTFSPathFromOSPath(path)
		return self.ntfs_delegate.LstatWithOSPath(ntfs_path)
	}
	return result, err
}

func init() {
	accessors.Register(&AutoFilesystemAccessor{})
}
