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

package zip

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	zipAccessorCurrentOpened = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accessor_zip_current_open",
		Help: "Number of currently opened ZIP files",
	})

	zipAccessorTotalOpened = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accessor_zip_total_open",
		Help: "Total Number of opened ZIP files",
	})

	zipAccessorCurrentReferences = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accessor_zip_current_references",
		Help: "Number of currently referenced ZIP files",
	})

	zipAccessorTotalTmpConversions = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accessor_zip_total_tmp_conversions",
		Help: "Total Number of opened ZIP files that we converted to tmp files",
	})

	zipAccessorCurrentTmpConversions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accessor_zip_current_tmp_conversions",
		Help: "Number of currently referenced ZIP files that exist in tmp files.",
	})
)

var (
	tracker = &Tracker{refs: make(map[string]int)}
)

type Tracker struct {
	mu   sync.Mutex
	refs map[string]int
}

func (self *Tracker) Inc(filename string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	prev, _ := self.refs[filename]
	self.refs[filename] = prev + 1
}

func (self *Tracker) Debug() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return fmt.Sprintf("%v\n", self.refs)
}

func (self *Tracker) Reset() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.refs = make(map[string]int)
}

func (self *Tracker) Dec(filename string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	prev, ok := self.refs[filename]
	if ok {
		prev--
		if prev == 0 {
			delete(self.refs, filename)
		} else {
			self.refs[filename] = prev
		}
	} else {
		panic(filename)
	}
}

// Wrapper around zip.File with reference counting. Note that each
// instance is holding a reference to the zip.Reader it came from. We
// also increase references to ZipFileCache to manage its references
type ZipFileInfo struct {
	member_file *zip.File
	_full_path  *accessors.OSPath
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

func (self *ZipFileInfo) Data() *ordereddict.Dict {
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
	return self._full_path.Basename()
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
	return self._full_path.String()
}

func (self *ZipFileInfo) OSPath() *accessors.OSPath {
	return self._full_path
}

func (self *ZipFileInfo) SetFullPath(full_path *accessors.OSPath) {
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

func (self *ZipFileInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Not implemented")
}

type _CDLookup struct {
	full_path   *accessors.OSPath
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
	fd accessors.ReadSeekCloser

	is_closed bool

	// Reference counting - all outstanding references to the zip
	// file. Make sure to call ZipFileCache.Close()
	refs int

	// An alternative lookup structure to fetch a zip.File (which will
	// be wrapped by a ZipFileInfo)
	lookup []_CDLookup

	zip_file_name string

	last_active time.Time

	id uint64
}

// Open a file within the cache. Find a direct reference to the
// zip.File object, and increase its reference. NOTE: The returned
// object must be closed to decrement the ZipFileCache reference
// count.
func (self *ZipFileCache) Open(full_path *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_active = time.Now()

	info, err := self._GetZipInfo(full_path)
	if err != nil {
		return nil, err
	}

	fd, err := info.member_file.Open()
	if err != nil {
		return nil, err
	}

	// We are leaking a zip.File out of our cache so we need to
	// increase our reference count.
	self.refs++
	zipAccessorCurrentReferences.Inc()
	return &SeekableZip{
		ReadCloser: fd,
		info:       info,
		full_path:  full_path,

		// We will be closed when done - Leak a reference.
		zip_file: self,
	}, nil
}

func (self *ZipFileCache) GetZipInfo(full_path *accessors.OSPath) (
	*ZipFileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._GetZipInfo(full_path)
}

// Searches our lookup table of components to zip.File objects, and
// wraps the zip.File object with a ZipFileInfo object.
func (self *ZipFileCache) _GetZipInfo(full_path *accessors.OSPath) (
	*ZipFileInfo, error) {

	// This is O(n) but due to the components length check it is
	// very fast.
loop:
	for _, cd_cache := range self.lookup {
		if len(full_path.Components) != len(cd_cache.full_path.Components) {
			continue
		}

		for j := range full_path.Components {
			if full_path.Components[j] != cd_cache.full_path.Components[j] {
				continue loop
			}
		}

		return &ZipFileInfo{
			member_file: cd_cache.member_file,
			_full_path:  full_path,
		}, nil
	}

	return nil, errors.New("Not found.")
}

func (self *ZipFileCache) GetChildren(
	full_path *accessors.OSPath) ([]*ZipFileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Determine if we already emitted this file.
	seen := make(map[string]bool)

	result := []*ZipFileInfo{}
loop:
	for _, cd_cache := range self.lookup {
		for j, component := range full_path.Components {
			if component != cd_cache.full_path.Components[j] {
				continue loop
			}
		}

		if len(cd_cache.full_path.Components) > len(full_path.Components) {
			member_name := cd_cache.full_path.Basename()
			member := &ZipFileInfo{
				_full_path: cd_cache.full_path,
			}

			// It is a file if the components are an exact match.
			if len(cd_cache.full_path.Components) == len(full_path.Components)+1 {
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

func (self *ZipFileCache) IncRef() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.refs++
	zipAccessorCurrentReferences.Inc()
}

func (self *ZipFileCache) CloseFile(full_path string) {
	self.Close()
}

func (self *ZipFileCache) IsClosed() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.is_closed
}

func (self *ZipFileCache) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.refs--
	zipAccessorCurrentReferences.Dec()
	if self.refs == 0 {
		self.fd.Close()
		self.is_closed = true
		zipAccessorCurrentOpened.Dec()
	}
}

// The ZipFileSystemAccessor is cached is a singleton cached in the
// root scope. We keep a list of most recently used cached of zip
// files for quick access.
type ZipFileSystemAccessor struct {
	mu       sync.Mutex
	fd_cache map[string]*ZipFileCache
	scope    vfilter.Scope
}

// Try to remove any file caches with no references.
func (self *ZipFileSystemAccessor) Trim() {
	self.mu.Lock()
	defer self.mu.Unlock()

	cache_size := vql_subsystem.GetIntFromRow(
		self.scope, self.scope, constants.ZIP_FILE_CACHE_SIZE)
	if cache_size == 0 {
		cache_size = 5
	}

	// Grow the cache up to max 10 elements.
	for key, fd := range self.fd_cache {
		if fd == nil {
			continue
		}

		if uint64(len(self.fd_cache)) > cache_size {
			fd.mu.Lock()
			refs := fd.refs
			fd.mu.Unlock()

			if refs == 1 {
				fd.Close()
			}
		}

		// Trim closed fd from our cache.
		if fd.IsClosed() {
			delete(self.fd_cache, key)
		}
	}
}

// Close all the items - called when root scope destroys
func (self *ZipFileSystemAccessor) CloseAll() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Close all elements
	for key, fd := range self.fd_cache {
		if fd != nil {
			fd.Close()
		}
		delete(self.fd_cache, key)
	}
}

// Returns a ZipFileCache wrapper around the zip.Reader. Be sure to
// close it when done. When the query completes, the zip file will be
// closed.
func (self *ZipFileSystemAccessor) GetZipFile(
	file_path string) (*ZipFileCache, *accessors.OSPath, error) {

	// Zip files typically use standard / path separators.
	full_path := accessors.NewLinuxOSPath(file_path)
	pathspec := full_path.PathSpec()

	base_pathspec := accessors.PathSpec{
		DelegateAccessor: pathspec.DelegateAccessor,
		DelegatePath:     pathspec.GetDelegatePath(),
	}
	cache_key := base_pathspec.String()

	for {
		self.mu.Lock()
		zip_file_cache, pres := self.fd_cache[cache_key]

		// The cached value is valid and ready - return it
		if pres &&
			zip_file_cache != nil &&
			!zip_file_cache.IsClosed() {
			zip_file_cache.IncRef()
			self.mu.Unlock()

			return zip_file_cache, full_path, nil
		}

		if !pres {
			// Store a nil in the map as a place holder, while we
			// build something.
			self.fd_cache[cache_key] = nil
			self.mu.Unlock()
			break
		}

		// Spin here until we have a real object to return
		self.mu.Unlock()
		time.Sleep(time.Millisecond)
	}

	accessor, err := accessors.GetAccessor(
		pathspec.DelegateAccessor, self.scope)
	if err != nil {
		self.scope.Log("%v: did you provide a URL or PathSpec?", err)
		delete(self.fd_cache, cache_key)
		return nil, nil, err
	}

	filename := pathspec.GetDelegatePath()
	fd, err := accessor.Open(filename)
	if err != nil {
		delete(self.fd_cache, cache_key)
		return nil, nil, err
	}

	reader, ok := fd.(io.ReaderAt)
	if !ok {
		self.scope.Log("file is not seekable")
		delete(self.fd_cache, cache_key)
		return nil, nil, errors.New("file is not seekable")
	}

	stat, err := accessor.Lstat(filename)
	if err != nil {
		self.scope.Log("Lstat: %v", err)
		delete(self.fd_cache, cache_key)
		return nil, nil, err
	}

	zip_file, err := zip.NewReader(reader, stat.Size())
	if err != nil {
		self.scope.Log("zip.NewReader: %v", err)
		delete(self.fd_cache, cache_key)
		return nil, nil, err
	}

	zipAccessorCurrentOpened.Inc()

	// Initial reference of 1 will be closed on scope destructor.
	zipAccessorCurrentReferences.Inc()
	zipAccessorTotalOpened.Inc()

	zip_file_cache := &ZipFileCache{
		zip_file: zip_file,
		fd:       fd,
		id:       utils.GetId(),

		// One reference to the scope.
		refs:          1,
		zip_file_name: base_pathspec.GetDelegatePath(),
	}

	for _, i := range zip_file.File {
		// Ignore directories which are signified by a
		// trailing / and have no content.
		if strings.HasSuffix(i.Name, "/") && i.UncompressedSize64 == 0 {
			continue
		}

		// Prepare the pathspec for each zip member. In order to
		// access the members, we need to open the currect zipfile (in
		// full_path) and open i.Name as the path.
		next_pathspec := full_path.PathSpec()
		next_pathspec.Path = i.Name

		next_item := _CDLookup{
			full_path:   full_path.Parse(next_pathspec.String()),
			member_file: i,
		}
		zip_file_cache.lookup = append(zip_file_cache.lookup, next_item)
	}

	// Set the new zip cache tracker in the fd cache.
	self.mu.Lock()

	// Leaking a zip file from this function, increase its reference -
	// callers have to close it.
	zip_file_cache.IncRef()

	// Replace the nil in the fd_cache with the real fd cache.
	self.fd_cache[cache_key] = zip_file_cache
	self.mu.Unlock()

	return zip_file_cache, full_path, nil
}

func (self *ZipFileSystemAccessor) Lstat(file_path string) (
	accessors.FileInfo, error) {
	if file_path == "" {
		utils.DlvBreak()
	}

	root, full_path, err := self.GetZipFile(file_path)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	return root.GetZipInfo(full_path)
}

func (self *ZipFileSystemAccessor) Open(filename string) (accessors.ReadSeekCloser, error) {
	// Fetch the zip file from cache again.
	zip_file_cache, full_path, err := self.GetZipFile(filename)
	if err != nil {
		return nil, err
	}
	defer zip_file_cache.Close()

	// Get the zip member from the zip file.
	return zip_file_cache.Open(full_path)
}

func (self *ZipFileSystemAccessor) ReadDir(file_path string) ([]accessors.FileInfo, error) {
	root, full_path, err := self.GetZipFile(file_path)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	children, err := root.GetChildren(full_path)
	if err != nil {
		return nil, err
	}

	result := []accessors.FileInfo{}
	for _, item := range children {
		result = append(result, item)
	}

	return result, nil
}

const (
	ZipFileSystemAccessorTag = "_ZipFS"
)

func (self ZipFileSystemAccessor) ParsePath(path string) *accessors.OSPath {
	return accessors.NewLinuxOSPath(path)
}

func (self *ZipFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	result_any := vql_subsystem.CacheGet(scope, ZipFileSystemAccessorTag)
	if result_any == nil {
		// Create a new cache in the scope.
		result := &ZipFileSystemAccessor{
			fd_cache: make(map[string]*ZipFileCache),
			scope:    scope,
		}
		vql_subsystem.CacheSet(scope, ZipFileSystemAccessorTag, result)

		vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			result.CloseAll()
		})
		return result, nil
	}

	res := result_any.(*ZipFileSystemAccessor)
	res.Trim()

	return res, nil
}

/* Zip members are normally compressed and therefore not seekable. If
   we read the members sequentially (e.g. for yara scanning or other
   sequential parsing), then there is no need to unpack the
   file. However, if the callers need to seek within the archive
   member we must unpack it to a tempfile.

   This wrapper manages this by wrapping the underlying zip member and
   unpacking to a tmpfile automatically depending on usage patterns.
*/
type SeekableZip struct {
	io.ReadCloser
	info   *ZipFileInfo
	offset int64

	full_path *accessors.OSPath

	// Hold a reference to the zip file itself.
	zip_file *ZipFileCache

	// If there is a tmp file backing the file, divert all IO to it.
	tmp_file_backing *os.File
}

func (self *SeekableZip) Close() error {
	// Remove the tmpfile now.
	if self.tmp_file_backing != nil {
		self.tmp_file_backing.Close()

		zipAccessorCurrentTmpConversions.Dec()
		os.Remove(self.tmp_file_backing.Name())
	}

	err := self.ReadCloser.Close()
	self.zip_file.Close()
	return err
}

func (self *SeekableZip) Read(buff []byte) (int, error) {
	if self.tmp_file_backing != nil {
		return self.tmp_file_backing.Read(buff)
	}

	n, err := self.ReadCloser.Read(buff)
	self.offset += int64(n)
	return n, err
}

// Comply with the ReadAt interface.
func (self *SeekableZip) ReadAt(buf []byte, offset int64) (int, error) {
	_, err := self.Seek(offset, 0)
	if err != nil {
		return 0, err
	}
	return self.Read(buf)
}

// Copy the member into a tmpfile.
func (self *SeekableZip) createTmpBackup() (err error) {
	// Make a fresh reader to the member so we are seeked to the
	// start of it.
	reader, err := self.zip_file.Open(self.full_path)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Create a tmp file to unpack the zip member into
	self.tmp_file_backing, err = ioutil.TempFile("", "zip*.tmp")
	if err != nil {
		return err
	}

	zipAccessorCurrentTmpConversions.Inc()
	zipAccessorTotalTmpConversions.Inc()

	_, err = io.Copy(self.tmp_file_backing, reader)
	if err != nil {
		return err
	}

	err = self.tmp_file_backing.Close()
	if err != nil {
		return err
	}

	// Reopen the file for reading.
	tmp_reader, err := os.Open(self.tmp_file_backing.Name())
	if err != nil {
		return err
	}

	self.tmp_file_backing = tmp_reader
	return nil
}

func (self *SeekableZip) Seek(offset int64, whence int) (int64, error) {
	if self.tmp_file_backing != nil {
		return self.tmp_file_backing.Seek(offset, whence)
	}

	switch whence {
	case io.SeekStart:
		if offset == 0 && self.offset == 0 {
			return 0, nil
		}

	}

	err := self.createTmpBackup()
	if err != nil {
		return 0, err
	}

	return self.Seek(offset, whence)
}

func init() {
	accessors.Register("zip", &ZipFileSystemAccessor{},
		`Open a zip file as if it was a directory.

Filename is a pathspec with a delegate accessor opening the Zip file,
and the Path representing the file within the zip file.

Example:

       select FullPath, Mtime, Size from glob(
         globs=pathspec(DelegateAccessor='file',
              DelegatePath="File.zip",
              Path='/**/*.txt'),
         accessor='zip')

`)

	json.RegisterCustomEncoder(&ZipFileInfo{}, accessors.MarshalGlobFileInfo)
}
