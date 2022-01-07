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
// A Raw NTFS accessor for disks.

// The NTFS accessor provides access to volumes, and Volume Shadow
// Copies through the VSS devices.

package filesystems

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_readers "www.velocidex.com/golang/velociraptor/vql/readers"
	"www.velocidex.com/golang/velociraptor/vql/windows/filesystems/readers"
	"www.velocidex.com/golang/vfilter"
)

const (
	// Scope cache tag for the NTFS parser
	NTFSFileSystemTag = "_NTFS"
)

type AccessorContext struct {
	mu sync.Mutex

	// The context is reference counted and will only be destroyed
	// when all users have closed it.
	refs          int
	cached_reader *ntfs.PagedReader
	cached_fd     *os.File
	is_closed     bool // Keep track if the file needs to be re-opened.
	ntfs_ctx      *ntfs.NTFSContext

	path_listing *cache.LRUCache
}

func (self *AccessorContext) GetNTFSContext() *ntfs.NTFSContext {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.ntfs_ctx
}

func (self *AccessorContext) IsClosed() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.is_closed
}

func (self *AccessorContext) IncRef() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.refs++
}

func (self *AccessorContext) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.refs--
	if self.refs <= 0 {
		self.cached_fd.Close()
		self.is_closed = true
	}
}

type NTFSFileInfo struct {
	info       *ntfs.FileInfo
	_full_path string
}

func (self *NTFSFileInfo) IsDir() bool {
	return self.info.IsDir
}

func (self *NTFSFileInfo) Size() int64 {
	return self.info.Size
}

func (self *NTFSFileInfo) Data() interface{} {
	result := ordereddict.NewDict().
		Set("mft", self.info.MFTId).
		Set("name_type", self.info.NameType).
		Set("fn_btime", self.info.FNBtime).
		Set("fn_mtime", self.info.FNMtime)
	if self.info.ExtraNames != nil {
		result.Set("extra_names", self.info.ExtraNames)
	}

	return result
}

func (self *NTFSFileInfo) Name() string {
	return self.info.Name
}

func (self *NTFSFileInfo) Sys() interface{} {
	return self.Data()
}

func (self *NTFSFileInfo) Mode() os.FileMode {
	var result os.FileMode = 0755
	if self.IsDir() {
		result |= os.ModeDir
	}
	return result
}

func (self *NTFSFileInfo) ModTime() time.Time {
	return self.info.Mtime
}

func (self *NTFSFileInfo) FullPath() string {
	return self._full_path
}

func (self *NTFSFileInfo) Btime() time.Time {
	return self.info.Btime
}

func (self *NTFSFileInfo) Mtime() time.Time {
	return self.info.Mtime
}

func (self *NTFSFileInfo) Ctime() time.Time {
	return self.info.Ctime
}

func (self *NTFSFileInfo) Atime() time.Time {
	return self.info.Atime
}

// Not supported
func (self *NTFSFileInfo) IsLink() bool {
	return false
}

func (self *NTFSFileInfo) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}

type NTFSFileSystemAccessor struct {
	scope  vfilter.Scope
	device string
}

func (self NTFSFileSystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	// Create a new cache in the scope.
	return &NTFSFileSystemAccessor{
		scope: scope,
	}, nil
}

func (self *NTFSFileSystemAccessor) getRootMFTEntry(ntfs_ctx *ntfs.NTFSContext) (
	*ntfs.MFT_ENTRY, error) {
	return ntfs_ctx.GetMFT(5)
}

func (self *NTFSFileSystemAccessor) ReadDir(path string) (res []glob.FileInfo, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			err, _ = r.(error)
		}
	}()

	result := []glob.FileInfo{}

	// The path must start with a valid device, otherwise we list
	// the devices.
	var device, subpath string

	if self.device != "" {
		device = self.device
		subpath = path
	} else {
		device, subpath, _ = self.GetRoot(path)
	}

	if err != nil {
		vss, err := discoverVSS()
		if err == nil {
			result = append(result, vss...)
		}

		logical, err := discoverLogicalDisks()
		if err == nil {
			result = append(result, logical...)
		}

		return result, nil
	}

	ntfs_ctx, err := readers.GetNTFSContext(self.scope, device)
	if err != nil {
		return nil, err
	}

	root, err := ntfs_ctx.GetMFT(5)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	dir, err := Open(self.scope, root, ntfs_ctx, device, subpath)
	if err != nil {
		return nil, err
	}

	// Only process each mft id once.
	seen := []int64{}
	in_seen := func(id int64) bool {
		for _, i := range seen {
			if i == id {
				return true
			}
		}
		return false
	}

	// List the directory.
	for _, node := range dir.Dir(ntfs_ctx) {
		node_mft_id := int64(node.MftReference())
		if in_seen(node_mft_id) {
			continue
		}

		seen = append(seen, node_mft_id)

		node_mft, err := ntfs_ctx.GetMFT(node_mft_id)
		if err != nil {
			continue
		}
		// Emit a result for each filename
		for _, info := range ntfs.Stat(ntfs_ctx, node_mft) {
			if info == nil {
				continue
			}
			full_path := device + subpath + "\\" + info.Name
			result = append(result, &NTFSFileInfo{
				info:       info,
				_full_path: full_path,
			})
		}
	}
	return result, nil
}

func (self *NTFSFileSystemAccessor) SetDataSource(dataSource string) {
	self.device = dataSource
}

func (self *NTFSFileSystemAccessor) GetRoot(path string) (
	device string, subpath string, err error) {
	if self.device != "" {
		return self.device, path, nil
	}
	return paths.GetDeviceAndSubpath(path)
}

type readAdapter struct {
	sync.Mutex

	info   glob.FileInfo
	reader ntfs.RangeReaderAt
	pos    int64
}

func (self *readAdapter) Ranges() []uploads.Range {
	result := []uploads.Range{}
	for _, rng := range self.reader.Ranges() {
		result = append(result, uploads.Range{
			Offset:   rng.Offset,
			Length:   rng.Length,
			IsSparse: rng.IsSparse,
		})
	}
	return result
}

func (self *readAdapter) Read(buf []byte) (res int, err error) {
	self.Lock()
	defer self.Unlock()

	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			err, _ = r.(error)
		}
	}()

	res, err = self.reader.ReadAt(buf, self.pos)
	self.pos += int64(res)

	// If ReadAt is unable to read anything it means an EOF.
	if res == 0 {
		return res, io.EOF
	}

	return res, err
}

func (self *readAdapter) ReadAt(buf []byte, offset int64) (int, error) {
	self.Lock()
	defer self.Unlock()
	self.pos = offset

	return self.reader.ReadAt(buf, offset)
}

func (self *readAdapter) Close() error {
	return nil
}

func (self *readAdapter) Stat() (os.FileInfo, error) {
	self.Lock()
	defer self.Unlock()

	return self.info, nil
}

func (self *readAdapter) Seek(offset int64, whence int) (int64, error) {
	self.Lock()
	defer self.Unlock()

	self.pos = offset
	return self.pos, nil
}

func (self *NTFSFileSystemAccessor) openRawDevice(device string) (res glob.ReadSeekCloser, err error) {
	// Find stats about this device
	infos, err := self.ReadDir("/*")
	if err != nil {
		return nil, err
	}

	var stat_info glob.FileInfo
	lower_device := strings.ToLower(device)

	for _, info := range infos {
		if lower_device == strings.ToLower(info.Name()) {
			stat_info = info
			break
		}
	}

	lru_size := vql_subsystem.GetIntFromRow(self.scope, self.scope, constants.NTFS_CACHE_SIZE)
	device_reader, err := vql_readers.NewPagedReader(
		self.scope, "raw_file", device, int(lru_size))
	return &readSeekReaderAdapter{reader: device_reader, info: stat_info}, err
}

func (self *NTFSFileSystemAccessor) Open(path string) (res glob.ReadSeekCloser, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			err, _ = r.(error)
		}
	}()

	// The path must start with a valid device, otherwise we list
	// the devices.
	device, subpath, err := self.GetRoot(path)
	if err != nil {
		return nil, errors.New("Unable to open raw device")
	}
	if subpath == "" {
		return self.openRawDevice(device)
	}

	components := self.PathSplit(subpath)

	ntfs_ctx, err := readers.GetNTFSContext(self.scope, device)
	if err != nil {
		return nil, err
	}

	root, err := self.getRootMFTEntry(ntfs_ctx)
	if err != nil {
		return nil, err
	}

	data, err := ntfs.GetDataForPath(ntfs_ctx, subpath)
	if err != nil {
		return nil, err
	}

	dirname := filepath.Dir(subpath)
	dir, err := Open(self.scope, root, ntfs_ctx, device, dirname)
	if err != nil {
		return nil, err
	}

	for _, info := range ntfs.ListDir(ntfs_ctx, dir) {
		if strings.ToLower(info.Name) == strings.ToLower(
			components[len(components)-1]) {
			return &readAdapter{
				info: &NTFSFileInfo{
					info:       info,
					_full_path: device + dirname + "\\" + info.Name,
				},
				reader: data,
			}, nil
		}
	}

	return nil, errors.New("File not found")
}

func (self *NTFSFileSystemAccessor) Lstat(path string) (res glob.FileInfo, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			err, _ = r.(error)
		}
	}()

	// The path must start with a valid device, otherwise we list
	// the devices.
	device, subpath, err := self.GetRoot(path)
	if err != nil {
		return nil, errors.New("Unable to open raw device")
	}

	components := self.PathSplit(subpath)

	ntfs_ctx, err := readers.GetNTFSContext(self.scope, device)
	if err != nil {
		return nil, err
	}

	root, err := self.getRootMFTEntry(ntfs_ctx)
	if err != nil {
		return nil, err
	}

	dirname := filepath.Dir(subpath)
	dir, err := Open(self.scope, root, ntfs_ctx, device, dirname)
	if err != nil {
		return nil, err
	}
	for _, info := range ntfs.ListDir(ntfs_ctx, dir) {
		if strings.ToLower(info.Name) == strings.ToLower(
			components[len(components)-1]) {
			res := &NTFSFileInfo{
				info:       info,
				_full_path: device + dirname + "\\" + info.Name,
			}
			return res, nil

		}
	}

	return nil, errors.New("File not found")
}

// We accept both / and \ as a path separator
var NTFSFileSystemAccessor_re = regexp.MustCompile("[\\\\/]")

func (self *NTFSFileSystemAccessor) PathSplit(path string) []string {
	return NTFSFileSystemAccessor_re.Split(path, -1)
}

func (self NTFSFileSystemAccessor) PathJoin(x, y string) string {
	return x + "\\" + strings.TrimLeft(y, "\\")
}

// We want to show the entire device as one name so we need to escape
// \\ characters so they are not interpreted as a path separator.
func escape(path string) string {
	result := strings.Replace(path, "\\", "%5c", -1)
	return strings.Replace(result, "/", "%2f", -1)
}

func unescape(path string) string {
	result := strings.Replace(path, "%5c", "\\", -1)
	return strings.Replace(result, "%2f", "/", -1)
}

// Open the MFT entry specified by a path name. Walks all directory
// indexes in the path to find the right MFT entry.
func Open(scope vfilter.Scope, self *ntfs.MFT_ENTRY,
	ntfs_ctx *ntfs.NTFSContext, device, filename string) (
	*ntfs.MFT_ENTRY, error) {

	components := utils.SplitComponents(filename)

	// Path is the relative path from the root of the device we want to list
	// component: The name of the file we want (case insensitive)
	// dir: The MFT entry to search.
	get_path_in_dir := func(path string, component string, dir *ntfs.MFT_ENTRY) (
		*ntfs.MFT_ENTRY, error) {

		key := device + path
		path_cache := GetNTFSPathCache(scope, device)
		item, pres := path_cache.GetComponentMetadata(key, component)
		if pres {
			return ntfs_ctx.GetMFT(item.MftId)
		}

		lru_map := make(map[string]*CacheMFT)

		// Populate the directory cache with all the mft ids.
		lower_component := strings.ToLower(component)
		for _, idx_record := range dir.Dir(ntfs_ctx) {
			file := idx_record.File()
			name_type := file.NameType().Name
			if name_type == "DOS" {
				continue
			}
			item_name := file.Name()
			mft_id := int64(idx_record.MftReference())

			lru_map[strings.ToLower(item_name)] = &CacheMFT{
				MftId:     mft_id,
				Component: item_name,
				NameType:  name_type,
			}
		}
		path_cache.SetLRUMap(key, lru_map)

		for _, v := range lru_map {
			if strings.ToLower(v.Component) == lower_component {
				return ntfs_ctx.GetMFT(v.MftId)
			}
		}

		return nil, errors.New("Not found")
	}

	// NOTE: This refreshes each parent directory in the LRU.
	directory := self
	path := ""
	for _, component := range components {
		if component == "" {
			continue
		}
		next, err := get_path_in_dir(
			path, component, directory)
		if err != nil {
			return nil, err
		}
		directory = next
		path = path + "\\" + component
	}

	return directory, nil
}

func init() {
	glob.Register("ntfs", &NTFSFileSystemAccessor{}, `Access the NTFS filesystem by parsing NTFS structures.`)

	json.RegisterCustomEncoder(&NTFSFileInfo{}, glob.MarshalGlobFileInfo)
}
