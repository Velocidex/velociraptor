/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2024 Rapid7 Inc.

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
	"github.com/Velocidex/ttlcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"www.velocidex.com/golang/regparser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
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
	key        *regparser.CM_KEY_NODE
	_full_path *accessors.OSPath
}

func (self *RawRegKeyInfo) IsDir() bool {
	return true
}

func (self *RawRegKeyInfo) Data() *ordereddict.Dict {
	return ordereddict.NewDict().Set("type", "Key")
}

func (self *RawRegKeyInfo) Size() int64 {
	return 0
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
	return self.key.Name()
}

func (self *RawRegKeyInfo) ModTime() time.Time {
	return self.key.LastWriteTime().Time
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
	value *regparser.CM_KEY_VALUE

	// The windows registry can store a value inside a reg key. This
	// makes the key act both as a directory and as a file
	// (i.e. ReadDir() will list the key) but Open() will read the
	// value.
	is_default_value bool

	_data *ordereddict.Dict
}

func (self *RawRegValueInfo) Name() string {
	return self.value.ValueName()
}

func (self *RawRegValueInfo) IsDir() bool {
	// We are also a key so act as a directory.
	return self.is_default_value
}

func (self *RawRegValueInfo) Mode() os.FileMode {
	if self.is_default_value {
		return 0755
	}
	return 0644
}

func (self *RawRegValueInfo) Size() int64 {
	return int64(self.value.DataSize())
}

func (self *RawRegValueInfo) Data() *ordereddict.Dict {
	if self._data != nil {
		return self._data
	}

	metricsReadValue.Inc()
	value_data := self.value.ValueData()
	value_type := self.value.TypeString()
	if self.is_default_value {
		value_type += "/Key"
	}
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
	info *RawRegValueInfo
}

func (self *RawValueBuffer) Close() error {
	return nil
}

func NewRawValueBuffer(buf string, stat *RawRegValueInfo) *RawValueBuffer {
	return &RawValueBuffer{
		bytes.NewReader(stat.value.ValueData().Data),
		stat,
	}
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

	lru         *ttlcache.Cache
	readdir_lru *ttlcache.Cache
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

	paged_reader, err := readers.NewPagedReader(
		scope, pathspec.DelegateAccessor, delegate, int(lru_size))
	if err != nil {
		scope.Log("%v: did you provide a URL or Pathspec?", err)
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

func (self *RawRegFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	my_lru := self.lru
	if my_lru == nil {
		my_lru = ttlcache.NewCache()
		my_lru.SetCacheSizeLimit(1000)
		my_lru.SetTTL(time.Minute)
	}

	my_readdir_lru := self.readdir_lru
	if my_readdir_lru == nil {
		my_readdir_lru = ttlcache.NewCache()
		my_readdir_lru.SetCacheSizeLimit(1000)
		my_readdir_lru.SetTTL(time.Minute)
	}

	return &RawRegFileSystemAccessor{
		scope: scope,
		root:  self.root,

		lru:         my_lru,
		readdir_lru: my_readdir_lru,
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

	cache_key := full_path.String()
	cached, err := self.readdir_lru.Get(cache_key)
	if err == nil {
		cached_res, ok := cached.(*readDirLRUItem)
		if ok {
			metricsReadDirLruHit.Inc()
			return cached_res.children, cached_res.err
		}
	}
	metricsReadDirLruMiss.Inc()

	// Cache the result of this function
	defer func() {
		self.readdir_lru.Set(cache_key, &readDirLRUItem{
			children: result,
			err:      err,
		})
	}()

	hive, err := getRegHive(self.scope, full_path)
	if err != nil {
		return nil, err
	}

	key := OpenKeyComponents(hive, full_path.Components)
	if key == nil {
		return nil, errors.New("Key not found")
	}

	seen := make(map[string]int)
	for idx, subkey := range key.Subkeys() {
		basename := subkey.Name()
		subkey := &RawRegKeyInfo{
			key:        subkey,
			_full_path: full_path.Append(basename),
		}
		seen[basename] = idx
		result = append(result, subkey)
	}

	for _, value := range key.Values() {
		basename := value.ValueName()
		value_obj := &RawRegValueInfo{
			RawRegKeyInfo: &RawRegKeyInfo{
				key:        key,
				_full_path: full_path.Append(basename),
			},
			value: value,
		}

		// Does this value have the same name as one of the keys?
		idx, pres := seen[basename]
		if pres {
			// Replace the old object with the value object
			value_obj.is_default_value = true
			result[idx] = value_obj
		} else {
			result = append(result, value_obj)
		}
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
			value_info.value.ValueData().Data, stat), nil
	}

	// Keys do not have any data.
	serialized, _ := json.Marshal(stat.Data)
	return NewValueBuffer(serialized, stat), nil
}

func (self *RawRegFileSystemAccessor) OpenWithOSPath(path *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {
	stat, err := self.LstatWithOSPath(path)
	if err != nil {
		return nil, err
	}

	value_info, ok := stat.(*RawRegValueInfo)
	if ok {
		return NewValueBuffer(
			value_info.value.ValueData().Data, stat), nil
	}

	// Keys do not have any data.
	serialized, _ := json.Marshal(stat.Data)
	return NewValueBuffer(serialized, stat), nil
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

	if len(full_path.Components) == 0 {
		return &accessors.VirtualFileInfo{
			Path:   full_path,
			IsDir_: true,
		}, nil
	}

	children, err := self.ReadDirWithOSPath(full_path.Dirname())
	if err != nil {
		return nil, err
	}

	name := full_path.Basename()
	for _, child := range children {
		if child.Name() == name {
			return child, nil
		}
	}

	return nil, errors.New("Key not found")
}

func init() {
	accessors.Register("raw_reg", &RawRegFileSystemAccessor{
		root: accessors.MustNewGenericOSPathWithBackslashSeparator(""),
	},
		`Access keys and values by parsing the raw registry hive. Path is a pathspec having delegate opening the raw registry hive.`)

	json.RegisterCustomEncoder(&RawRegKeyInfo{}, accessors.MarshalGlobFileInfo)
	json.RegisterCustomEncoder(&RawRegValueInfo{}, accessors.MarshalGlobFileInfo)
}
