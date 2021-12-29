/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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

package filesystem

import (
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"

	"www.velocidex.com/golang/velociraptor/glob"
)

type GzipFileInfo struct {
	_modtime   time.Time
	_name      string
	_full_path string
}

func (self *GzipFileInfo) IsDir() bool {
	return false
}

func (self *GzipFileInfo) Size() int64 {
	// We dont really know the size.
	return -1
}

func (self *GzipFileInfo) Data() interface{} {
	result := ordereddict.NewDict()
	return result
}

func (self *GzipFileInfo) Name() string {
	return self._name
}

func (self *GzipFileInfo) Sys() interface{} {
	return self.Data()
}

func (self *GzipFileInfo) Mode() os.FileMode {
	return 0644
}

func (self *GzipFileInfo) ModTime() time.Time {
	return self._modtime
}

func (self *GzipFileInfo) FullPath() string {
	return self._full_path
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

func (self *GzipFileInfo) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}

type ReaderStat interface {
	glob.ReadSeekCloser
	LStat() (glob.FileInfo, error)
}

type GzipFileSystemAccessor struct {
	scope  vfilter.Scope
	getter FileGetter
}

// This method splits the path string into a root component (which the
// glob should start from) and a path component (Which is used by the
// glob algorithm).

// In our case the path string looks something like:
//
// file:///tmp/foo.zip#/dir/name.txt
//
// so the root is file:///tmp/foo.zip# and the path is /dir/name.txt
func (self *GzipFileSystemAccessor) GetRoot(path string) (string, string, error) {
	pathspec, err := glob.PathSpecFromString(path)
	if err != nil {
		return "", "", err
	}

	Fragment := pathspec.Path
	pathspec.Path = ""

	return pathspec.String(), Fragment, nil
}

func (self *GzipFileSystemAccessor) Lstat(file_path string) (glob.FileInfo, error) {
	seekablegzip, err := self.getter(file_path, self.scope)
	if err != nil {
		return nil, err
	}
	defer seekablegzip.Close()

	return seekablegzip.LStat()
}

func (self *GzipFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	return self.getter(path, self.scope)
}

var GzipFileSystemAccessor_re = regexp.MustCompile("/")

func (self *GzipFileSystemAccessor) PathSplit(path string) []string {
	return GzipFileSystemAccessor_re.Split(path, -1)
}

// The root is a url for the parent node and the stem is the new subdir.
// Example: root  is file://path/to/zip#subdir and stem is foo ->
// file://path/to/zip#subdir/foo
func (self *GzipFileSystemAccessor) PathJoin(root, stem string) string {
	pathspec, err := glob.PathSpecFromString(root)
	if err != nil {
		path.Join(root, stem)
	}

	pathspec.Path = path.Join(pathspec.Path, stem)

	return pathspec.String()
}

func (self *GzipFileSystemAccessor) ReadDir(file_path string) ([]glob.FileInfo, error) {
	return nil, nil
}

func (self GzipFileSystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	return &GzipFileSystemAccessor{
		scope: scope, getter: self.getter}, nil
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

func (self *SeekableGzip) Stat() (os.FileInfo, error) {
	return self.info, nil
}

func (self *SeekableGzip) LStat() (glob.FileInfo, error) {
	return self.info, nil
}

// Any getter that implements this can be used
type FileGetter func(file_path string, scope vfilter.Scope) (ReaderStat, error)

func GetBzip2File(file_path string, scope vfilter.Scope) (ReaderStat, error) {
	pathspec, err := glob.PathSpecFromString(file_path)
	if err != nil {
		return nil, err
	}

	accessor, err := glob.GetAccessor(pathspec.DelegateAccessor, scope)
	if err != nil {
		scope.Log("%v: did you provide a URL or PathSpec?", err)
		return nil, err
	}

	fd, err := accessor.Open(pathspec.GetDelegatePath())
	if err != nil {
		return nil, err
	}

	stat, err := fd.Stat()
	if err != nil {
		return nil, err
	}

	zr := bzip2.NewReader(fd)
	return &SeekableGzip{reader: fd,
		gz: ioutil.NopCloser(zr),
		info: &GzipFileInfo{
			_modtime:   stat.ModTime(),
			_name:      stat.Name(),
			_full_path: file_path,
		}}, nil
}

func GetGzipFile(file_path string, scope vfilter.Scope) (ReaderStat, error) {
	pathspec, err := glob.PathSpecFromString(file_path)
	if err != nil {
		return nil, err
	}

	accessor, err := glob.GetAccessor(pathspec.DelegateAccessor, scope)
	if err != nil {
		scope.Log("%v: did you provide a URL or PathSpec?", err)
		return nil, err
	}

	fd, err := accessor.Open(pathspec.GetDelegatePath())
	if err != nil {
		return nil, err
	}

	stat, err := fd.Stat()
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
				_full_path: file_path,
			}}, nil
	}

	return &SeekableGzip{reader: fd,
		gz: zr,
		info: &GzipFileInfo{
			_modtime:   zr.ModTime,
			_name:      stat.Name(),
			_full_path: file_path,
		}}, nil
}

func init() {
	glob.Register("gzip", &GzipFileSystemAccessor{
		getter: GetGzipFile}, `Access the content of gzip files. The filename is a pathspec with a delegate accessor opening the actual gzip file.`)
	glob.Register("bzip2", &GzipFileSystemAccessor{
		getter: GetBzip2File}, `Access the content of gzip files. The filename is a pathspec with a delegate accessor opening the actual gzip file.`)

	json.RegisterCustomEncoder(&GzipFileInfo{}, glob.MarshalGlobFileInfo)
}
