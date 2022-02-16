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
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
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
	return self._full_path.Copy()
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
		return nil, fmt.Errorf("While reading %v %s: %w",
			utils.DebugString(info.member_file),
			full_path.String(), err)
	}

	// We are leaking a zip.File out of our cache so we need to
	// increase our reference count.
	self.refs++
	zipAccessorCurrentReferences.Inc()
	return &SeekableZip{
		delegate:  fd,
		info:      info,
		full_path: full_path,

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
			_full_path:  full_path.Copy(),
		}, nil
	}

	return nil, errors.New("Not found.")
}

func (self *ZipFileCache) GetChildren(
	full_path *accessors.OSPath) ([]*ZipFileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Determine if we already emitted this file.
	seen := make(map[string]*ZipFileInfo)

loop:
	for _, cd_cache := range self.lookup {
		// This breaks if the cd component does not have the same
		// prefix as required.
		for j, component := range full_path.Components {
			if component != cd_cache.full_path.Components[j] {
				continue loop
			}
		}

		// The required directory depth we need.
		depth := len(full_path.Components)
		if len(cd_cache.full_path.Components) < depth {
			continue
		}

		// Get the part of the path that is at the required depth.
		member_name := cd_cache.full_path.Components[depth]

		// Have we seen this before?
		old_result, pres := seen[member_name]
		if pres {
			// Only show the first real file (Zip files can have many
			// data files with the same name).
			if old_result.member_file != nil {
				continue
			}

			if len(cd_cache.full_path.Components) != depth+1 {
				continue
			}
		}

		// It is a file if the components are an exact match.
		if len(cd_cache.full_path.Components) == depth+1 {
			seen[member_name] = &ZipFileInfo{
				_full_path:  cd_cache.full_path.Copy(),
				member_file: cd_cache.member_file,
			}

			// A directory has no member file
		} else {
			seen[member_name] = &ZipFileInfo{
				_full_path: full_path.Append(member_name),
			}
		}
	}

	result := make([]*ZipFileInfo, 0, len(seen))
	for _, v := range seen {
		result = append(result, v)
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

/* Zip members are normally compressed and therefore not seekable. If
   we read the members sequentially (e.g. for yara scanning or other
   sequential parsing), then there is no need to unpack the
   file. However, if the callers need to seek within the archive
   member we must unpack it to a tempfile.

   This wrapper manages this by wrapping the underlying zip member and
   unpacking to a tmpfile automatically depending on usage patterns.
*/
type SeekableZip struct {
	mu sync.Mutex

	delegate io.ReadCloser
	info     *ZipFileInfo
	offset   int64

	full_path *accessors.OSPath

	// Hold a reference to the zip file itself.
	zip_file *ZipFileCache

	// If there is a tmp file backing the file, divert all IO to it.
	tmp_file_backing *os.File

	closed bool
}

func (self *SeekableZip) Close() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Remove the tmpfile now.
	if self.tmp_file_backing != nil {
		self.tmp_file_backing.Close()

		zipAccessorCurrentTmpConversions.Dec()
		os.Remove(self.tmp_file_backing.Name())
	}

	err := self.delegate.Close()
	self.zip_file.Close()
	self.closed = true
	return err
}

func (self *SeekableZip) DebugString() string {
	if self.tmp_file_backing != nil {
		return fmt.Sprintf("SeekableZip of %v backed on %v",
			self.full_path.String(), self.tmp_file_backing.Name())
	}
	return fmt.Sprintf("SeekableZip of %v, closed: %v",
		self.full_path.String(), self.closed)
}

func (self *SeekableZip) Read(buff []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.read(buff)
}

func (self *SeekableZip) read(buff []byte) (int, error) {
	if self.tmp_file_backing != nil {
		nn, err := self.tmp_file_backing.Read(buff)
		self.offset += int64(nn)
		return nn, err
	}

	n, err := self.delegate.Read(buff)
	self.offset += int64(n)
	return n, err
}

// Comply with the ReadAt interface.
func (self *SeekableZip) ReadAt(buf []byte, offset int64) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	_, err := self.seek(offset, 0)
	if err != nil {
		return 0, err
	}
	n, err := self.read(buf)
	return n, err
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
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.seek(offset, whence)
}

func (self *SeekableZip) seek(offset int64, whence int) (int64, error) {
	if self.tmp_file_backing != nil {
		current_offset, err := self.tmp_file_backing.Seek(offset, whence)
		if err != nil {
			self.offset = current_offset
		}
		return current_offset, err
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

	current_offset, err := self.tmp_file_backing.Seek(offset, whence)
	if err != nil {
		self.offset = current_offset
	}
	return current_offset, err
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
