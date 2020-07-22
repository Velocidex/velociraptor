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

// A Zip accessor.

// This accessor provides access to compressed archives. The filename
// is encoded in such a way that this accessor can delegate to another
// accessor to actually open the underlying zip file. This makes it
// possible to open zip files read through e.g. raw ntfs.

// For example a filename is URL encoded as:
// ntfs:/c:\\Windows\\File.zip#/foo/bar

// Refers to the file opened by the accessor "ntfs" (The URL Scheme)
// with a path (URL Path) of c:\\Windows\File.zip. We then open this
// file and return a member called /foo/bar (The URL Fragment) within
// the archive.

// This scheme allows us to nest zip files if we need to:
// zip://fs:%2Fc:%5Cfoo%5Cbar%23nested.zip#foo/bar

// Refers to the file /foo/bar stored within a zip file nested.zip
// which is itself stored on the filesystem at c:\foo\bar\nested.zip

package filesystem

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"

	"www.velocidex.com/golang/velociraptor/glob"
)

type ZipFileInfo struct {
	info       *zip.File
	_name      string
	_full_path string
}

func (self *ZipFileInfo) IsDir() bool {
	return self.info == nil
}

func (self *ZipFileInfo) Size() int64 {
	if self.info == nil {
		return 0
	}

	return int64(self.info.UncompressedSize64)
}

func (self *ZipFileInfo) Data() interface{} {
	result := ordereddict.NewDict()
	if self.info != nil {
		result.Set("CompressedSize", self.info.CompressedSize64)
		switch self.info.Method {
		case 0:
			result.Set("Method", "stored")
		case 8:
			result.Set("Method", "zlib")
		default:
			result.Set("Method", "unknown")
		}
	}

	return result
}

func (self *ZipFileInfo) Name() string {
	return self._name
}

func (self *ZipFileInfo) Sys() interface{} {
	return self.Data()
}

func (self *ZipFileInfo) Mode() os.FileMode {
	var result os.FileMode = 0755
	if self.IsDir() {
		result |= os.ModeDir
	}
	return result
}

func (self *ZipFileInfo) ModTime() time.Time {
	if self.info != nil {
		return self.info.Modified
	}
	return time.Unix(0, 0)
}

func (self *ZipFileInfo) FullPath() string {
	return self._full_path
}

func (self *ZipFileInfo) Mtime() utils.TimeVal {
	if self.info != nil {
		return utils.TimeVal{
			Sec: self.info.Modified.Unix(),
		}
	}

	return utils.TimeVal{
		Sec: 0,
	}
}

func (self *ZipFileInfo) Ctime() utils.TimeVal {
	return self.Mtime()
}

func (self *ZipFileInfo) Atime() utils.TimeVal {
	return self.Mtime()
}

// Not supported
func (self *ZipFileInfo) IsLink() bool {
	return false
}

func (self *ZipFileInfo) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}

func (self *ZipFileInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Size     int64
		Mode     os.FileMode
		ModeStr  string
		ModTime  time.Time
		Sys      interface{}
		Mtime    utils.TimeVal
		Ctime    utils.TimeVal
		Atime    utils.TimeVal
	}{
		FullPath: self.FullPath(),
		Size:     self.Size(),
		Mode:     self.Mode(),
		ModeStr:  self.Mode().String(),
		ModTime:  self.ModTime(),
		Sys:      self.Sys(),
		Mtime:    self.Mtime(),
		Ctime:    self.Ctime(),
		Atime:    self.Atime(),
	})

	return result, err
}

type _CDLookup struct {
	components []string
	info       *zip.File
}

type ZipFileCache struct {
	zip_file *zip.Reader
	fd       glob.ReadSeekCloser
	lookup   []_CDLookup
}

type ZipFileSystemAccessor struct {
	mu       sync.Mutex
	fd_cache map[string]*ZipFileCache
	scope    *vfilter.Scope
}

func (self *ZipFileSystemAccessor) GetZipFile(
	file_path string) (*ZipFileCache, *url.URL, error) {
	url, err := url.Parse(file_path)
	if err != nil {
		return nil, nil, err
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	base_url := *url
	base_url.Fragment = ""

	zip_file_cache, pres := self.fd_cache[base_url.String()]
	if !pres {
		accessor, err := glob.GetAccessor(url.Scheme, self.scope)
		if err != nil {
			return nil, nil, err
		}

		fd, err := accessor.Open(url.Path)
		if err != nil {
			return nil, nil, err
		}

		reader, ok := fd.(io.ReaderAt)
		if !ok {
			return nil, nil, errors.New("file is not seekable")
		}

		stat, err := fd.Stat()
		if err != nil {
			return nil, nil, err
		}

		zip_file, err := zip.NewReader(reader, stat.Size())
		if err != nil {
			return nil, nil, err
		}

		zip_file_cache = &ZipFileCache{
			zip_file: zip_file,
			fd:       fd,
		}

		self.fd_cache[url.String()] = zip_file_cache

		for _, i := range zip_file.File {
			file_path := path.Clean(i.Name)
			zip_file_cache.lookup = append(zip_file_cache.lookup,
				_CDLookup{
					components: strings.Split(file_path, "/"),
					info:       i,
				})
		}
	}

	return zip_file_cache, url, nil

}

// This method splits the path string into a root component (which the
// glob should start from) and a path component (Which is used by the
// glob algorithm).

// In our case the path string looks something like:
//
// file:///tmp/foo.zip#/dir/name.txt
//
// so the root is file:///tmp/foo.zip# and the path is /dir/name.txt
func (self *ZipFileSystemAccessor) GetRoot(path string) (string, string, error) {
	url, err := url.Parse(path)
	if err != nil {
		return "", "", err
	}

	Fragment := url.Fragment
	url.Fragment = ""

	return url.String() + "#", Fragment, nil
}

func (self *ZipFileSystemAccessor) Lstat(file_path string) (glob.FileInfo, error) {
	root, url, err := self.GetZipFile(file_path)
	if err != nil {
		return nil, err
	}

	zip_path := path.Clean(path.Join("/", url.Fragment))

	components := []string{}
	for _, i := range strings.Split(zip_path, "/") {
		if i != "" {
			components = append(components, i)
		}
	}

loop:
	for _, cd_cache := range root.lookup {
		if len(components) != len(cd_cache.components) {
			continue
		}

		for j := range components {
			if components[j] != cd_cache.components[j] {
				continue loop
			}
		}

		return &ZipFileInfo{
			info:       cd_cache.info,
			_name:      components[len(components)-1],
			_full_path: url.String(),
		}, nil
	}

	return nil, errors.New("Not found.")
}

func (self *ZipFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	info_generic, err := self.Lstat(path)
	if err != nil {
		return nil, err
	}

	info := info_generic.(*ZipFileInfo)

	fd, err := info.info.Open()
	if err != nil {
		return nil, err
	}

	return &SeekableZip{ReadCloser: fd, info: info}, nil
}

var ZipFileSystemAccessor_re = regexp.MustCompile("/")

func (self *ZipFileSystemAccessor) PathSplit(path string) []string {
	return ZipFileSystemAccessor_re.Split(path, -1)
}

// The root is a url for the parent node and the stem is the new subdir.
// Example: root  is file://path/to/zip#subdir and stem is foo ->
// file://path/to/zip#subdir/foo
func (self *ZipFileSystemAccessor) PathJoin(root, stem string) string {
	url, err := url.Parse(root)
	if err != nil {
		path.Join(root, stem)
	}

	url.Fragment = path.Join(url.Fragment, stem)

	result := url.String()

	return result
}

func (self *ZipFileSystemAccessor) ReadDir(file_path string) ([]glob.FileInfo, error) {
	root, url, err := self.GetZipFile(file_path)
	if err != nil {
		return nil, err
	}

	zip_path := path.Clean(path.Join("/", url.Fragment))

	components := []string{}
	for _, i := range strings.Split(zip_path, "/") {
		if i != "" {
			components = append(components, i)
		}
	}

	result := []glob.FileInfo{}

	// Determine if we already emitted this file. O(n) but if n is
	// small it should be faster than map.
	name_in_result := func(name string) bool {
		for _, item := range result {
			if item.Name() == name {
				return true
			}
		}
		return false
	}

loop:
	for _, cd_cache := range root.lookup {
		for j := range components {
			if components[j] != cd_cache.components[j] {
				continue loop
			}
		}

		if len(cd_cache.components) > len(components) {
			// member is either a directory or a file.
			member_name := cd_cache.components[len(components)]

			url.Fragment = path.Join(
				cd_cache.components[:len(components)+1]...)

			member := &ZipFileInfo{
				_name:      member_name,
				_full_path: url.String(),
			}

			// It is a file if the components are an exact match.
			if len(cd_cache.components) == len(components)+1 {
				member.info = cd_cache.info
			}

			if !name_in_result(member_name) {
				result = append(result, member)
			}
		}
	}

	return result, nil
}

const (
	ZipFileSystemAccessorTag = "_ZipFS"
)

func (self ZipFileSystemAccessor) New(scope *vfilter.Scope) (glob.FileSystemAccessor, error) {
	result_any := vql_subsystem.CacheGet(scope, ZipFileSystemAccessorTag)
	if result_any == nil {
		// Create a new cache in the scope.
		result := &ZipFileSystemAccessor{
			fd_cache: make(map[string]*ZipFileCache),
			scope:    scope,
		}

		vql_subsystem.CacheSet(scope, ZipFileSystemAccessorTag, result)

		// When scope is destroyed, we close all the filehandles.
		scope.AddDestructor(func() {
			for _, v := range result.fd_cache {
				v.fd.Close()
			}
		})
		return result, nil
	}

	return result_any.(glob.FileSystemAccessor), nil
}

type SeekableZip struct {
	io.ReadCloser
	info   *ZipFileInfo
	offset int64
}

func (self *SeekableZip) Read(buff []byte) (int, error) {
	n, err := self.ReadCloser.Read(buff)
	self.offset += int64(n)
	return n, err
}

func (self *SeekableZip) Seek(offset int64, whence int) (int64, error) {
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

func (self *SeekableZip) Stat() (os.FileInfo, error) {
	return self.info, nil
}

func init() {
	glob.Register("zip", &ZipFileSystemAccessor{})
}
