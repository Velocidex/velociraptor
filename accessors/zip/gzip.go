/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

// A GZip accessor.

// This accessor provides access to compressed archives. The filename
// is encoded in such a way that this accessor can delegate to another
// accessor to actually open the underlying zip file. This makes it
// possible to open zip files read through e.g. raw ntfs.

// For example a filename is URL encoded as:
// ntfs:/c:\\Windows\\File.gz

// Refers to the file opened by the accessor "ntfs" (The URL Scheme)
// with a path (URL Path) of c:\\Windows\File.gz.

package zip

import (
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

type GzipFileInfo struct {
	_modtime   time.Time
	_name      string
	_full_path *accessors.OSPath
}

func (self *GzipFileInfo) IsDir() bool {
	return false
}

func (self *GzipFileInfo) Size() int64 {
	// We dont really know the size.
	return -1
}

func (self *GzipFileInfo) Data() *ordereddict.Dict {
	result := ordereddict.NewDict()
	return result
}

func (self *GzipFileInfo) Name() string {
	return self._name
}

func (self *GzipFileInfo) Mode() os.FileMode {
	return 0644
}

func (self *GzipFileInfo) ModTime() time.Time {
	return self._modtime
}

func (self *GzipFileInfo) FullPath() string {
	return self._full_path.String()
}

func (self *GzipFileInfo) OSPath() *accessors.OSPath {
	return self._full_path.Copy()
}

func (self *GzipFileInfo) Mtime() time.Time {
	return self._modtime
}

func (self *GzipFileInfo) Btime() time.Time {
	return self._modtime
}

func (self *GzipFileInfo) Ctime() time.Time {
	return self._modtime
}

func (self *GzipFileInfo) Atime() time.Time {
	return self._modtime
}

// Not supported
func (self *GzipFileInfo) IsLink() bool {
	return false
}

func (self *GzipFileInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Not implemented")
}

type ReaderStat interface {
	accessors.ReadSeekCloser
	LStat() (accessors.FileInfo, error)
}

type GzipFileSystemAccessor struct {
	scope  vfilter.Scope
	getter FileGetter

	root *accessors.OSPath
}

func (self *GzipFileSystemAccessor) Lstat(file_path string) (
	accessors.FileInfo, error) {

	full_path, err := self.ParsePath(file_path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self *GzipFileSystemAccessor) LstatWithOSPath(
	file_path *accessors.OSPath) (
	accessors.FileInfo, error) {
	seekablegzip, err := self.getter(file_path, self.scope)
	if err != nil {
		return nil, err
	}
	defer seekablegzip.Close()

	return seekablegzip.LStat()
}

func (self *GzipFileSystemAccessor) Open(file_path string) (
	accessors.ReadSeekCloser, error) {
	full_path, err := self.ParsePath(file_path)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self *GzipFileSystemAccessor) OpenWithOSPath(path *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {
	return self.getter(path, self.scope)
}

func (self *GzipFileSystemAccessor) ReadDir(file_path string) (
	[]accessors.FileInfo, error) {
	return nil, nil
}

func (self *GzipFileSystemAccessor) ReadDirWithOSPath(
	full_path *accessors.OSPath) ([]accessors.FileInfo, error) {
	return nil, nil
}

func (self GzipFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return self.root.Parse(path)
}

func (self GzipFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "gzip",
		Description: `Access the content of gzip files. The filename is a pathspec with a delegate accessor opening the actual gzip file.`,
	}
}

func (self GzipFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	return &GzipFileSystemAccessor{
		scope:  scope,
		getter: self.getter,
		root:   self.root,
	}, nil
}

func NewGzipFileSystemAccessor(
	root *accessors.OSPath, getter FileGetter) *GzipFileSystemAccessor {
	return &GzipFileSystemAccessor{root: root, getter: getter}
}

type SeekableGzip struct {
	reader io.ReadCloser
	gz     io.ReadCloser
	info   *GzipFileInfo
	offset int64
}

func (self *SeekableGzip) Close() error {
	self.gz.Close()
	return self.reader.Close()
}

func (self *SeekableGzip) Read(buff []byte) (int, error) {
	n, err := self.gz.Read(buff)
	self.offset += int64(n)
	return n, err
}

func (self *SeekableGzip) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		if offset == 0 && self.offset == 0 {
			return 0, nil
		}

	}
	return 0, fmt.Errorf(
		"Seeking to %v (%v) not supported on compressed files.",
		offset, whence)
}

func (self *SeekableGzip) LStat() (accessors.FileInfo, error) {
	return self.info, nil
}

// Any getter that implements this can be used
type FileGetter func(full_path *accessors.OSPath,
	scope vfilter.Scope) (ReaderStat, error)

func GetBzip2File(full_path *accessors.OSPath, scope vfilter.Scope) (
	ReaderStat, error) {
	pathspec := full_path.PathSpec()

	// The gzip accessor must use a delegate but if one is not
	// provided we use the "auto" accessor, to open the underlying
	// file.
	if pathspec.DelegateAccessor == "" && pathspec.DelegatePath == "" {
		pathspec.DelegatePath = pathspec.Path
		pathspec.DelegateAccessor = "auto"
	}

	accessor, err := accessors.GetAccessor(pathspec.DelegateAccessor, scope)
	if err != nil {
		scope.Log("%v: did you provide a URL or PathSpec?", err)
		return nil, err
	}

	delegate_path := pathspec.GetDelegatePath()
	fd, err := accessor.Open(delegate_path)
	if err != nil {
		return nil, err
	}

	stat, err := accessor.Lstat(delegate_path)
	if err != nil {
		return nil, err
	}

	zr := bzip2.NewReader(fd)
	return &SeekableGzip{reader: fd,
		gz: ioutil.NopCloser(zr),
		info: &GzipFileInfo{
			_modtime:   stat.ModTime(),
			_name:      stat.Name(),
			_full_path: full_path.Copy(),
		}}, nil
}

func GetGzipFile(full_path *accessors.OSPath, scope vfilter.Scope) (ReaderStat, error) {
	pathspec := full_path.PathSpec()

	// The gzip accessor must use a delegate but if one is not
	// provided we use the "auto" accessor, to open the underlying
	// file.
	if pathspec.DelegateAccessor == "" && pathspec.GetDelegatePath() == "" {
		pathspec.DelegatePath = pathspec.Path
		pathspec.DelegateAccessor = "auto"
	}

	accessor, err := accessors.GetAccessor(pathspec.DelegateAccessor, scope)
	if err != nil {
		scope.Log("%v: did you provide a PathSpec?", err)
		return nil, err
	}

	delegate_path := pathspec.GetDelegatePath()
	fd, err := accessor.Open(delegate_path)
	if err != nil {
		return nil, err
	}

	stat, err := accessor.Lstat(delegate_path)
	if err != nil {
		return nil, err
	}

	zr, err := gzip.NewReader(fd)
	if err != nil {
		// Try to seek the file back
		_, err = fd.Seek(0, io.SeekStart)
		if err != nil {
			// If it does not work - reopen the file.
			fd.Close()
			fd, err = accessor.Open(pathspec.GetDelegatePath())
			if err != nil {
				return nil, err
			}
		}

		// Not a gzip file but we open it anyway.
		return &SeekableGzip{reader: fd,
			gz: fd,
			info: &GzipFileInfo{
				_modtime:   stat.ModTime(),
				_name:      stat.Name(),
				_full_path: full_path.Copy(),
			}}, nil
	}

	return &SeekableGzip{reader: fd,
		gz: zr,
		info: &GzipFileInfo{
			_modtime:   zr.ModTime,
			_name:      stat.Name(),
			_full_path: full_path.Copy(),
		}}, nil
}

func init() {
	accessors.Register(NewGzipFileSystemAccessor(
		accessors.MustNewLinuxOSPath(""), GetGzipFile),
	)

	accessors.Register(accessors.DescribeAccessor(
		NewGzipFileSystemAccessor(
			accessors.MustNewLinuxOSPath(""), GetBzip2File),
		accessors.AccessorDescriptor{
			Name:        "bzip2",
			Description: `Access the content of gzip files. The filename is a pathspec with a delegate accessor opening the actual gzip file.`,
		}))

	json.RegisterCustomEncoder(&GzipFileInfo{}, accessors.MarshalGlobFileInfo)
}
