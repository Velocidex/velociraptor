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
// A filesystem accessor for accessing registry hives through raw
// file parsing.

// We make the registry look like a filesystem:
//  1. Keys are mapped as directories, and values are files.
//  2. The file is interpreted as a URL with the following format:
//     accessor:/path#key_path
//  3. We use the accessor and path to open the underlying file, then
//     extract the key or value named by the key_path from it.
//  4. Normalized paths contain / for directory separators.
//  5. Normalized paths have rawreg: prefix.
package raw_registry

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"www.velocidex.com/golang/regparser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	"www.velocidex.com/golang/vfilter"
)

const (
	MAX_EMBEDDED_REG_VALUE = 4 * 1024
)

var (
	metricsReadValue = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "rawreg_getvalue",
			Help: "Number of time we Queried Value from the registry",
		})

	metricsReadDirLruHit = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "rawreg_readdir_lru_hit",
			Help: "Performance of the Read Dir Cache",
		})

	metricsReadDirLruMiss = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "rawreg_readdir_lru_miss",
			Help: "Performance of the Read Dir Cache",
		})
)

type RawRegKeyInfo struct {
	mu sync.Mutex

	_full_path *accessors.OSPath
	_data      *ordereddict.Dict
	_modtime   time.Time

	_key *regparser.CM_KEY_NODE
}

func (self *RawRegKeyInfo) IsDir() bool {
	return true
}

func (self *RawRegKeyInfo) Data() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self._data == nil {
		self._data = ordereddict.NewDict().Set("type", "Key")
	}

	return self._data
}

func (self *RawRegKeyInfo) Size() int64 {
	return 0
}

func (self *RawRegKeyInfo) UniqueName() string {
	// Key names can not have \ in them so it is safe to add this
	// without risk of collisions.
	return self._full_path.String() + "\\"
}

func (self *RawRegKeyInfo) FullPath() string {
	return self._full_path.String()
}

func (self *RawRegKeyInfo) OSPath() *accessors.OSPath {
	return self._full_path
}

func (self *RawRegKeyInfo) Mode() os.FileMode {
	return 0755 | os.ModeDir
}

func (self *RawRegKeyInfo) Name() string {
	return self._full_path.Basename()
}

func (self *RawRegKeyInfo) ModTime() time.Time {
	return self._modtime
}

func (self *RawRegKeyInfo) Mtime() time.Time {
	return self.ModTime()
}

func (self *RawRegKeyInfo) Ctime() time.Time {
	return self.Mtime()
}

func (self *RawRegKeyInfo) Btime() time.Time {
	return self.Mtime()
}

func (self *RawRegKeyInfo) Atime() time.Time {
	return self.Mtime()
}

// Not supported
func (self *RawRegKeyInfo) IsLink() bool {
	return false
}

func (self *RawRegKeyInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Not implemented")
}

func (self *RawRegKeyInfo) UnmarshalJSON(data []byte) error {
	return nil
}

type RawRegValueInfo struct {
	// Containing key
	*RawRegKeyInfo

	// Hold a reference so value can be decoded lazily.
	_value *regparser.CM_KEY_VALUE

	// Once value is decoded once it will be cached here.
	_data *ordereddict.Dict
	_size int64
}

func (self *RawRegValueInfo) Copy() *RawRegValueInfo {
	return &RawRegValueInfo{
		RawRegKeyInfo: &RawRegKeyInfo{
			_full_path: self._full_path,
			_modtime:   self._modtime,
			_key:       self._key,
		},
		_value: self._value,
		_data:  self._data,
		_size:  self._size,
	}
}

func (self *RawRegValueInfo) IsDir() bool {
	return false
}

func (self *RawRegValueInfo) UniqueName() string {
	return self._full_path.String()
}

func (self *RawRegValueInfo) Mode() os.FileMode {
	return 0644
}

func (self *RawRegValueInfo) Size() int64 {
	if self._size > 0 {
		return self._size
	}
	self._size = int64(self._value.DataSize())
	return self._size
}

func (self *RawRegValueInfo) Data() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self._data != nil {
		return self._data
	}

	metricsReadValue.Inc()
	value_data := self._value.ValueData()
	value_type := self._value.TypeString()
	result := ordereddict.NewDict().
		Set("type", value_type).
		Set("data_len", len(value_data.Data))

	switch value_data.Type {
	case regparser.REG_SZ, regparser.REG_EXPAND_SZ:
		result.Set("value", strings.TrimRight(value_data.String, "\x00"))

	case regparser.REG_MULTI_SZ:
		result.Set("value", value_data.MultiSz)

	case regparser.REG_DWORD, regparser.REG_QWORD, regparser.REG_DWORD_BIG_ENDIAN:
		result.Set("value", value_data.Uint64)
	default:
		if len(value_data.Data) < MAX_EMBEDDED_REG_VALUE {
			result.Set("value", value_data.Data)
		}
	}

	self._data = result
	return result
}

type RawValueBuffer struct {
	*bytes.Reader
}

type rawHiveCache struct {
	mu sync.Mutex

	// Maintain a cache of already parsed hives
	hive_cache map[string]*regparser.Registry
}

func (self *rawHiveCache) Get(name string) (*regparser.Registry, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, ok := self.hive_cache[name]
	return res, ok
}

func (self *rawHiveCache) Set(name string, reg *regparser.Registry) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.hive_cache[name] = reg
}

type RawRegFileSystemAccessor struct {
	scope vfilter.Scope
	root  *accessors.OSPath

	cache *RawRegFileSystemAccessorCache
}

// Registery filesystems are usually case insensitive.
func (self RawRegFileSystemAccessor) GetCanonicalFilename(
	path *accessors.OSPath) string {
	return strings.ToLower(path.String())
}

func getRegHiveCache(scope vfilter.Scope) *rawHiveCache {
	result_any := vql_subsystem.CacheGet(scope, RawRegFileSystemTag)
	if result_any != nil {
		cached, ok := result_any.(*rawHiveCache)
		if ok {
			return cached
		}
	}

	result := &rawHiveCache{
		hive_cache: make(map[string]*regparser.Registry),
	}
	vql_subsystem.CacheSet(scope, RawRegFileSystemTag, result)

	return result
}

func getRegHive(scope vfilter.Scope,
	file_path *accessors.OSPath) (*regparser.Registry, error) {

	// Cache the parsed hive under the underlying file.
	pathspec := file_path.PathSpec()
	base_pathspec := accessors.PathSpec{
		DelegateAccessor: pathspec.DelegateAccessor,
		DelegatePath:     pathspec.GetDelegatePath(),
	}
	cache_key := base_pathspec.String()

	hive_cache := getRegHiveCache(scope)
	reg, pres := hive_cache.Get(cache_key)
	if pres {
		return reg, nil
	}

	lru_size := vql_subsystem.GetIntFromRow(
		scope, scope, constants.RAW_REG_CACHE_SIZE)

	delegate, err := file_path.Delegate(scope)
	if err != nil {
		return nil, err
	}

	paged_reader, err := readers.NewAccessorReader(
		scope, pathspec.DelegateAccessor, delegate, int(lru_size))
	if err != nil {
		scope.Log("%v: did you provide a Pathspec?", err)
		return nil, err
	}

	// Make sure we can read the header so we can propagate errors
	// properly.
	header := make([]byte, 4)
	_, err = paged_reader.ReadAt(header, 0)
	if err != nil {
		paged_reader.Close()
		return nil, err
	}

	hive, err := regparser.NewRegistry(paged_reader)
	if err != nil {
		paged_reader.Close()
		return nil, err
	}

	hive_cache.Set(cache_key, hive)

	return hive, nil
}

const RawRegFileSystemTag = "_RawReg"

func (self RawRegFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "raw_reg",
		Description: `Access keys and values by parsing the raw registry hive. Path is a pathspec having delegate opening the raw registry hive.`,
	}
}

func (self *RawRegFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	return &RawRegFileSystemAccessor{
		scope: scope,
		root:  self.root,
		cache: getRegFileSystemAccessorCache(scope),
	}, nil
}

// Raw Registry paths a just just generic paths:
// 1. Separator can be / or \ when specified.
// 2. Path are always serialized with /
// 3. No required hive at first element.
// 4. Paths start with / since they refer to the root of the raw hive file.
func (self RawRegFileSystemAccessor) ParsePath(path string) (*accessors.OSPath, error) {
	return self.root.Parse(path)
}

func (self *RawRegFileSystemAccessor) ReadDir(key_path string) (
	[]accessors.FileInfo, error) {

	full_path, err := self.ParsePath(key_path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self *RawRegFileSystemAccessor) ReadDirWithOSPath(
	full_path *accessors.OSPath) (result []accessors.FileInfo, err error) {

	contents, _, err := self._readDirWithOSPath(full_path)
	return contents, err
}

// Return all the contents in the directory including all keys and all
// values, even if some keys have a default value.
// Additionally returns the CM_KEY_NODE for this actual directory.

// This function is recursive! It ascends to the root cell recursively
// and resolves all keys along the path to the required key. On each
// level the function tries the LRU to avoid further recursion. This
// means that in practice most of the time we wont actually be
// recursing more than a few levels because top level keys will be
// cached in the LRU.
func (self *RawRegFileSystemAccessor) _readDirWithOSPath(
	full_path *accessors.OSPath) (result []accessors.FileInfo, key *regparser.CM_KEY_NODE, err error) {

	cache_key := full_path.String()
	cached, err := self.cache.readdir_lru.Get(cache_key)
	if err == nil {
		cached_res, ok := cached.(*readDirLRUItem)
		if ok {
			metricsReadDirLruHit.Inc()
			return cached_res.children, cached_res.key, cached_res.err
		}
	}
	metricsReadDirLruMiss.Inc()

	// Cache the result of this function
	defer func() {
		err1 := self.cache.readdir_lru.Set(cache_key, &readDirLRUItem{
			children: result,
			err:      err,
			key:      key,
		})
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	// Listing the top level of the hive.
	if len(full_path.Components) == 0 {
		hive, err := getRegHive(self.scope, full_path)
		if err != nil {
			return nil, nil, err
		}

		root_cell := hive.Profile.HCELL(hive.Reader,
			0x1000+int64(hive.BaseBlock.RootCell()))

		nk := root_cell.KeyNode()
		if nk != nil {
			listing, err := self._readDirFromKey(full_path, nk)
			return listing, nk, err
		}
		return nil, nil, utils.NotFoundError
	}

	parent := full_path.Dirname()
	basename := full_path.Basename()

	// If the directory is not cached, get its parent and list it.
	contents, key, err := self._readDirWithOSPath(parent)
	if err != nil {
		return nil, nil, err
	}

	// Find the required key in the parent directory listing.
	for _, item := range contents {
		key, ok := item.(*RawRegKeyInfo)
		if !ok {
			continue
		}

		// Found it!
		if key._key != nil &&
			strings.EqualFold(key.Name(), basename) {
			listing, err := self._readDirFromKey(full_path, key._key)
			return listing, key._key, err
		}
	}

	return nil, nil, utils.NotFoundError
}

func (self *RawRegFileSystemAccessor) _readDirFromKey(
	parent *accessors.OSPath, key *regparser.CM_KEY_NODE) (
	result []accessors.FileInfo, err error) {

	subkeys := key.Subkeys()
	for _, subkey := range subkeys {
		basename := subkey.Name()
		subkey := &RawRegKeyInfo{
			_full_path: parent.Append(basename),
			_modtime:   subkey.LastWriteTime().Time,
			_key:       subkey,
		}
		result = append(result, subkey)
	}

	// All Values carry their mode time as the parent key
	key_mod_time := key.LastWriteTime().Time
	for _, value := range key.Values() {
		basename := value.ValueName()
		if basename == "" {
			basename = "@"
		}
		value_obj := &RawRegValueInfo{
			RawRegKeyInfo: &RawRegKeyInfo{
				_full_path: parent.Append(basename),
				_modtime:   key_mod_time,
			},
			_value: value,
		}
		result = append(result, value_obj)
	}
	return result, nil
}

func (self *RawRegFileSystemAccessor) Open(path string) (
	accessors.ReadSeekCloser, error) {
	stat, err := self.Lstat(path)
	if err != nil {
		return nil, err
	}

	value_info, ok := stat.(*RawRegValueInfo)
	if ok {
		return NewValueBuffer(
			value_info._value.ValueData().Data, stat), nil
	}

	// Keys do not have any data.
	serialized, _ := json.Marshal(stat.Data)
	return NewValueBuffer(serialized, stat), nil
}

func (self *RawRegFileSystemAccessor) OpenWithOSPath(path *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {
	stats, err := self.multiLstat(path)
	if err != nil {
		return nil, err
	}

	// We are looking for a value to open try to find one but if now,
	// just serialize the key data.
	for _, stat := range stats {
		value_info, ok := stat.(*RawRegValueInfo)
		if ok {
			return NewValueBuffer(
				value_info._value.ValueData().Data, stat), nil
		}
	}

	// Keys do not have any data.
	serialized, _ := json.Marshal(stats[0].Data)
	return NewValueBuffer(serialized, stats[0]), nil
}

func (self *RawRegFileSystemAccessor) Lstat(filename string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self *RawRegFileSystemAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (
	accessors.FileInfo, error) {

	// Top level stat
	if len(full_path.Components) == 0 {
		return &accessors.VirtualFileInfo{
			Path:   full_path,
			IsDir_: true,
		}, nil
	}

	res, err := self.multiLstat(full_path)
	if err != nil {
		return nil, err
	}

	// Return the first one.
	return res[0], nil
}

// The registry can have keys and values named the same so an Lstat
// can actually return two separate entities. This function returns
// both.
func (self *RawRegFileSystemAccessor) multiLstat(
	full_path *accessors.OSPath) (res []accessors.FileInfo, err error) {

	name := full_path.Basename()
	container := full_path.Dirname()

	children, err := self.ReadDirWithOSPath(container)
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		child_name := child.Name()

		// Fetch default value as either @ or ""
		if strings.EqualFold(child_name, name) ||
			(name == "@" && child_name == "") {
			res = append(res, child)
		}
	}

	if len(res) == 0 {
		return nil, errors.New("Key not found")
	}

	return res, nil
}

func init() {
	accessors.Register(&RawRegFileSystemAccessor{
		root: accessors.MustNewGenericOSPathWithBackslashSeparator(""),
	})

	json.RegisterCustomEncoder(&RawRegKeyInfo{}, accessors.MarshalGlobFileInfo)
	json.RegisterCustomEncoder(&RawRegValueInfo{}, accessors.MarshalGlobFileInfo)
}
