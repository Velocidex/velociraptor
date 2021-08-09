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
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"

	"www.velocidex.com/golang/velociraptor/glob"
)

// Wrapper around zip.File with reference counting. Note that each
// instance is holding a reference to the zip.Reader it came from. We
// also increase references to ZipFileCache to manage its references
type ZipFileInfo struct {
	member_file *zip.File
	_name       string
	_full_path  string
}

func (self *ZipFileInfo) IsDir() bool {
	return self.member_file == nil
}

func (self *ZipFileInfo) Size() int64 {
	if self.member_file == nil {
		return 0
	}

	return int64(self.member_file.UncompressedSize64)
}

func (self *ZipFileInfo) Data() interface{} {
	result := ordereddict.NewDict()
	if self.member_file != nil {
		result.Set("CompressedSize", self.member_file.CompressedSize64)
		switch self.member_file.Method {
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
	if self.member_file != nil {
		return self.member_file.Modified
	}
	return time.Unix(0, 0)
}

func (self *ZipFileInfo) FullPath() string {
	return self._full_path
}

func (self *ZipFileInfo) SetFullPath(full_path string) {
	self._full_path = full_path
}

func (self *ZipFileInfo) Mtime() time.Time {
	if self.member_file != nil {
		return self.member_file.Modified
	}

	return time.Time{}
}

func (self *ZipFileInfo) Ctime() time.Time {
	return self.Mtime()
}

func (self *ZipFileInfo) Btime() time.Time {
	return self.Mtime()
}

func (self *ZipFileInfo) Atime() time.Time {
	return self.Mtime()
}

// Not supported
func (self *ZipFileInfo) IsLink() bool {
	return false
}

func (self *ZipFileInfo) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}

type _CDLookup struct {
	components  []string
	member_file *zip.File
}

// A Reference counter around zip.Reader. Each zip.File that is
// released to external code via Open() is wrapped by ZipFileInfo and
// the reference count increases. When the references are exhausted
// the reader will be closed as well as its underlying file.
type ZipFileCache struct {
	mu       sync.Mutex
	zip_file *zip.Reader

	// Underlying file - will be closed when the references are zero.
	fd glob.ReadSeekCloser

	is_closed bool

	// Reference counting - all outstanding references to the zip
	// file. Make sure to call ZipFileCache.Close()
	refs int64

	// An alternative lookup structure to fetch a zip.File (which will
	// be wrapped by a ZipFileInfo)
	lookup []_CDLookup
}

// Open a file within the cache. Find a direct reference to the
// zip.File object, and increase its reference. NOTE: The returned
// object must be closed to decrement the ZipFileCache reference
// count.
func (self *ZipFileCache) Open(
	components []string, full_path string) (glob.ReadSeekCloser, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	info, err := self._GetZipInfo(components, full_path)
	if err != nil {
		return nil, err
	}

	fd, err := info.member_file.Open()
	if err != nil {
		return nil, err
	}

	// We are leaking a zip.File out of our cache so we need to
	// increase our reference count.
	atomic.AddInt64(&self.refs, 1)

	return &SeekableZip{
		ReadCloser: fd,
		info:       info,

		// We will be closed when done.
		zip_file: self,
	}, nil
}

func (self *ZipFileCache) GetZipInfo(
	components []string, full_path string) (*ZipFileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._GetZipInfo(components, full_path)
}

// Searches our lookup table of components to zip.File objects, and
// wraps the zip.File object with a ZipFileInfo object.
func (self *ZipFileCache) _GetZipInfo(
	components []string, full_path string) (*ZipFileInfo, error) {

	// This is O(n) but due to the components length check it is
	// very fast.
loop:
	for _, cd_cache := range self.lookup {
		if len(components) != len(cd_cache.components) {
			continue
		}

		for j := range components {
			if components[j] != cd_cache.components[j] {
				continue loop
			}
		}

		return &ZipFileInfo{
			member_file: cd_cache.member_file,
			_name:       components[len(components)-1],
			_full_path:  full_path,
		}, nil
	}

	return nil, errors.New("Not found.")
}

func (self *ZipFileCache) GetChildren(components []string) ([]*ZipFileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Determine if we already emitted this file.
	seen := make(map[string]bool)

	result := []*ZipFileInfo{}
loop:
	for _, cd_cache := range self.lookup {
		for j := range components {
			if components[j] != cd_cache.components[j] {
				continue loop
			}
		}

		if len(cd_cache.components) > len(components) {
			// member is either a directory or a file.
			member_name := cd_cache.components[len(components)]

			member := &ZipFileInfo{
				_name: member_name,
			}

			// It is a file if the components are an exact match.
			if len(cd_cache.components) == len(components)+1 {
				member.member_file = cd_cache.member_file
			}

			_, pres := seen[member_name]
			if !pres {
				result = append(result, member)
				seen[member_name] = true
			}
		}
	}
	return result, nil
}

func (self *ZipFileCache) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	atomic.AddInt64(&self.refs, -1)
	if self.refs == 0 {
		self.fd.Close()
		self.is_closed = true
	}
}

type ZipFileSystemAccessor struct {
	mu       sync.Mutex
	fd_cache map[string]*ZipFileCache
	scope    vfilter.Scope
}

// Returns a ZipFileCache wrapper around the zip.Reader. Be sure to
// close it when done. When the query completes, the zip file will be
// closed.
func (self *ZipFileSystemAccessor) GetZipFile(
	file_path string) (*ZipFileCache, *url.URL, error) {
	url, err := url.Parse(file_path)
	if err != nil {
		return nil, nil, err
	}

	base_url := *url
	base_url.Fragment = ""

	self.mu.Lock()
	zip_file_cache, pres := self.fd_cache[base_url.String()]
	self.mu.Unlock()

	if !pres || zip_file_cache.is_closed {
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

			// One reference to the scope.
			refs: 1,
		}

		for _, i := range zip_file.File {
			// Ignore directories which are signified by a
			// trailing / and have no content.
			if strings.HasSuffix(i.Name, "/") && i.UncompressedSize64 == 0 {
				continue
			}
			zip_file_cache.lookup = append(zip_file_cache.lookup,
				_CDLookup{
					components:  fragmentToComponents(i.Name),
					member_file: i,
				})
		}
		self.mu.Lock()
		self.fd_cache[url.String()] = zip_file_cache
		self.mu.Unlock()
	}

	atomic.AddInt64(&zip_file_cache.refs, 1)

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

func fragmentToComponents(fragment string) []string {
	components := []string{}
	for _, i := range strings.Split(fragment, "/") {
		if i != "" {
			components = append(components, i)
		}
	}
	return components
}

func (self *ZipFileSystemAccessor) Lstat(file_path string) (glob.FileInfo, error) {
	root, url, err := self.GetZipFile(file_path)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	return root.GetZipInfo(fragmentToComponents(url.Fragment), file_path)
}

func (self *ZipFileSystemAccessor) Open(filename string) (glob.ReadSeekCloser, error) {
	// Fetch the zip file from cache again.
	zip_file_cache, url, err := self.GetZipFile(filename)
	if err != nil {
		return nil, err
	}

	// Get the zip member from the zip file.
	fd, err := zip_file_cache.Open(
		fragmentToComponents(url.Fragment), filename)
	if err != nil {
		zip_file_cache.Close()
		return nil, err
	}
	return fd, nil
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
	defer root.Close()

	children, err := root.GetChildren(
		fragmentToComponents(url.Fragment))
	if err != nil {
		return nil, err
	}

	result := []glob.FileInfo{}
	for _, item := range children {
		// Make a copy
		child_url := *url
		child_url.Fragment = path.Join(child_url.Fragment, item.Name())
		item.SetFullPath(child_url.String())
		result = append(result, item)
	}

	return result, nil
}

const (
	ZipFileSystemAccessorTag = "_ZipFS"
)

func (self *ZipFileSystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	result_any := vql_subsystem.CacheGet(scope, ZipFileSystemAccessorTag)
	if result_any == nil {
		// Create a new cache in the scope.
		result := &ZipFileSystemAccessor{
			fd_cache: make(map[string]*ZipFileCache),
			scope:    scope,
		}

		vql_subsystem.CacheSet(scope, ZipFileSystemAccessorTag, result)

		// When scope is destroyed, we close all the filehandles.
		err := vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			// Decrement refs until we are allowed to
			// close the file.
			for _, v := range result.fd_cache {
				v.mu.Lock()
				atomic.AddInt64(&v.refs, -1)
				if v.refs == 0 {
					v.fd.Close()
				}
				v.mu.Unlock()
			}
		})
		return result, err
	}

	return result_any.(glob.FileSystemAccessor), nil
}

type SeekableZip struct {
	io.ReadCloser
	info   *ZipFileInfo
	offset int64

	// Hold a reference to the zip file itself.
	zip_file *ZipFileCache
}

func (self *SeekableZip) Close() error {
	err := self.ReadCloser.Close()
	self.zip_file.Close()
	return err
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

	json.RegisterCustomEncoder(&ZipFileInfo{}, glob.MarshalGlobFileInfo)
}
