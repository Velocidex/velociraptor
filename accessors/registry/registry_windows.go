//go:build windows
// +build windows

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
// A filesystem accessor for accessing the windows registry.

// We make the registry look like a filesystem:
// 1. Keys are mapped as directories, and values are files.
// 2. Map the root path to a virtual directory containing all the root keys.
// 3. Normalized paths contain / for directory separators.
// 4. Normalized paths have reg: prefix.
package registry

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"golang.org/x/sys/windows/registry"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	// Include some common aliases.
	root_keys = ordereddict.NewDict(). //  map[string]registry.Key{
			Set("HKEY_CLASSES_ROOT", registry.CLASSES_ROOT).
			Set("HKEY_CURRENT_USER", registry.CURRENT_USER).
			Set("HKEY_LOCAL_MACHINE", registry.LOCAL_MACHINE).
			Set("HKEY_USERS", registry.USERS).
			Set("HKEY_CURRENT_CONFIG", registry.CURRENT_CONFIG).
			Set("HKEY_PERFORMANCE_DATA", registry.PERFORMANCE_DATA)

	// Values smaller than this will be included in the stat entry
	// itself.
	MAX_EMBEDDED_REG_VALUE = 4 * 1024

	metricsReadValue = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_getvalue",
			Help: "Number of time we Queried Value from the registry",
		})

	metricsAccessorReadValue = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_accessor_getvalue",
			Help: "Number of time we Queried Value from the accessor",
		})

	metricsLruHit = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_keyinfo_lru_hit",
			Help: "Performance of the Key Info Cache",
		})

	metricsLruMiss = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_keyinfo_lru_miss",
			Help: "Performance of the Key Info Cache",
		})

	metricsReadDirLruHit = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_readdir_lru_hit",
			Help: "Performance of the Read Dir Cache",
		})

	metricsReadDirLruMiss = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_readdir_lru_miss",
			Help: "Performance of the Read Dir Cache",
		})

	metricsOpen = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_open",
			Help: "Total number of Open operations",
		})

	metricsStat = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_stat",
			Help: "Total number of Lstat operations",
		})

	metricsOpenKey = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "registry_openkey",
			Help: "Total number of RegOpenKey operations",
		})
)

func GetHiveFromName(name string) (registry.Key, bool) {
	hive, pres := root_keys.Get(name)
	if pres {
		return hive.(registry.Key), pres
	}
	return registry.CLASSES_ROOT, false
}

type RegKeyInfo struct {
	_modtime   time.Time
	_full_path *accessors.OSPath
	_data      *ordereddict.Dict
}

func (self *RegKeyInfo) IsDir() bool {
	return true
}

func (self *RegKeyInfo) Data() *ordereddict.Dict {
	if self._data == nil {
		return ordereddict.NewDict()
	}
	return self._data
}

func (self *RegKeyInfo) Size() int64 {
	return 0
}

func (self *RegKeyInfo) UniqueName() string {
	// Key names can not have \ in them so it is safe to add this
	// without risk of collisions.
	return self._full_path.String() + "\\"
}

func (self *RegKeyInfo) FullPath() string {
	return self._full_path.String()
}

func (self *RegKeyInfo) OSPath() *accessors.OSPath {
	return self._full_path.Copy()
}

func (self *RegKeyInfo) Mode() os.FileMode {
	return 0755 | os.ModeDir
}

func (self *RegKeyInfo) Name() string {
	return self._full_path.Basename()
}

func (self *RegKeyInfo) ModTime() time.Time {
	return self._modtime
}

func (self *RegKeyInfo) Mtime() time.Time {
	return self.ModTime()
}

func (self *RegKeyInfo) Btime() time.Time {
	return self.Mtime()
}

func (self *RegKeyInfo) Ctime() time.Time {
	return self.Mtime()
}

func (self *RegKeyInfo) Atime() time.Time {
	return self.Mtime()
}

// Not supported
func (self *RegKeyInfo) IsLink() bool {
	return false
}

func (self *RegKeyInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Not implemented")
}

func (u *RegKeyInfo) UnmarshalJSON(data []byte) error {
	return nil
}

type RegValueInfo struct {
	RegKeyInfo
	Type  string
	_size int64

	// A private copy of the value data. This is not made
	// available to VQL. The data made available to VQL will be
	// attached to the Data field of the FileInfo struct. While
	// that can only contain fields smaller than
	// MAX_EMBEDDED_REG_VALUE, we store the full value in the
	// _binary_data field. We can then return the buffer for an
	// Open() call.
	_binary_data []byte
}

func (self *RegValueInfo) IsDir() bool {
	return false
}

func (self *RegValueInfo) UniqueName() string {
	return self._full_path.String()
}

func (self *RegValueInfo) Mode() os.FileMode {
	return 0755
}

func (self *RegValueInfo) Data() *ordereddict.Dict {
	metricsAccessorReadValue.Inc()

	self.materialize()
	return self._data
}

func (self *RegValueInfo) Size() int64 {
	self.materialize()

	return self._size
}

// RegValueInfo are lazy structures that only materialize themselves
// just in time.
func (self *RegValueInfo) materialize() error {
	// Use self._data as indicator if the structure is materialized.
	if self._data != nil {
		return nil
	}

	// Last component is the value name
	value_name := self._full_path.Basename()
	full_key_path := self._full_path.Dirname()

	// Internally we represent the default value of a key as the name
	// '@'
	if value_name == "@" {
		value_name = ""
	}

	hive, key_path, err := getHiveAndKey(full_key_path)
	if err != nil {
		// Cache the error
		self._data = ordereddict.NewDict().Set("Error", err.Error())
		return err
	}

	metricsOpenKey.Inc()
	key, err := registry.OpenKey(hive, key_path,
		registry.READ|registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		// Cache the error
		self._data = ordereddict.NewDict().Set("Error", err.Error())
		return err
	}
	defer key.Close()

	buf_size, value_type, value, err := getValue(key, value_name)
	if err != nil {
		// Cache the error
		self._data = ordereddict.NewDict().Set("Error", err.Error())
		return err
	}

	self._size = int64(buf_size)

	switch value_type {
	case registry.DWORD, registry.DWORD_BIG_ENDIAN, registry.QWORD:
		switch value_type {
		case registry.DWORD_BIG_ENDIAN:
			self.Type = "DWORD_BIG_ENDIAN"

		case registry.DWORD:
			self.Type = "DWORD"

		case registry.QWORD:
			self.Type = "QWORD"
		}

		self._data = ordereddict.NewDict().
			Set("type", self.Type).
			Set("value", value)

	case registry.BINARY:
		if buf_size < MAX_EMBEDDED_REG_VALUE {
			self._data = ordereddict.NewDict().
				Set("type", "BINARY").
				Set("value", value)
		}
		value_bytes, _ := value.([]byte)
		self._binary_data = value_bytes
		self.Type = "BINARY"

	case registry.MULTI_SZ:
		self._binary_data, _ = json.Marshal(value)
		self.Type = "MULTI_SZ"

		if buf_size < MAX_EMBEDDED_REG_VALUE {
			self._data = ordereddict.NewDict().
				Set("type", "MULTI_SZ").
				Set("value", value)
		}

	case registry.SZ, registry.EXPAND_SZ:
		switch value_type {
		case registry.SZ:
			self.Type = "SZ"

		case registry.EXPAND_SZ:
			self.Type = "EXPAND_SZ"
		}

		value_str, _ := value.(string)
		self._binary_data = []byte(value_str)

		if buf_size < MAX_EMBEDDED_REG_VALUE {
			self._data = ordereddict.NewDict().
				Set("type", self.Type).

				// We do not expand the value data because this will
				// depend on the agent's own environment strings.
				Set("value", value)
		}

	default:
		value_bytes, _ := value.([]byte)
		self._binary_data = value_bytes
		self.Type = fmt.Sprintf("%d", value_type)

		if buf_size < MAX_EMBEDDED_REG_VALUE {
			self._data = ordereddict.NewDict().
				Set("type", self.Type).
				Set("value", value)
		} else {
			self._data = ordereddict.NewDict().
				Set("type", self.Type).
				Set("value", "<Binary Data>")
		}
	}

	return nil
}

type ValueBuffer struct {
	io.ReadSeeker
	info accessors.FileInfo
}

func (self *ValueBuffer) Stat() (accessors.FileInfo, error) {
	return self.info, nil
}

func (self *ValueBuffer) Close() error {
	return nil
}

func NewValueBuffer(buf []byte, stat accessors.FileInfo) *ValueBuffer {
	return &ValueBuffer{
		bytes.NewReader(buf),
		stat,
	}
}

type RegFileSystemAccessor struct {
	cache *RegFileSystemAccessorCache
}

func (self RegFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "registry",
		Description: `Access the registery like a filesystem using the OS APIs.`,
	}
}

// Registery filesystems are usually case insensitive.
func (self RegFileSystemAccessor) GetCanonicalFilename(
	path *accessors.OSPath) string {
	return strings.ToLower(path.String())
}

func (self *RegFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	return &RegFileSystemAccessor{
		cache: getRegFileSystemAccessorCache(scope),
	}, nil
}

func (self RegFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewWindowsRegistryPath(path)
}

func (self RegFileSystemAccessor) ReadDir(path string) (
	[]accessors.FileInfo, error) {

	full_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self RegFileSystemAccessor) ReadDirWithOSPath(
	full_path *accessors.OSPath) (result []accessors.FileInfo, err error) {

	cache_key := full_path.String()
	cached, ok := self.cache.GetDir(cache_key)
	if ok {
		return cached.children, cached.err
	}

	// Cache the result of this function
	defer func() {
		self.cache.SetDir(cache_key, &readDirLRUItem{
			children: result,
			err:      err,
			age:      utils.GetTime().Now(),
		})
	}()

	// Root directory is just the name of the hives.
	if len(full_path.Components) == 0 {
		for _, k := range root_keys.Keys() {
			result = append(result, &accessors.VirtualFileInfo{
				IsDir_: true,
				Path:   full_path.Append(k),
				Data_: ordereddict.NewDict().
					Set("type", "hive"),
			})
		}
		return result, nil
	}

	hive, key_path, err := getHiveAndKey(full_path)
	if err != nil {
		return nil, err
	}

	metricsOpenKey.Inc()
	key, err := registry.OpenKey(hive, key_path,
		registry.READ|registry.QUERY_VALUE|
			registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return nil, err
	}
	defer key.Close()

	// Now enumerate the subkeys
	subkeys, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}

	for _, subkey_name := range subkeys {
		key_info, ok := self.cache.Get(full_path.Append(subkey_name).String())
		if ok {
			result = append(result, key_info)
			continue
		}

		// Not in cache, we need to add it
		metricsOpenKey.Inc()
		subkey, err := registry.OpenKey(key, subkey_name,
			registry.READ|registry.QUERY_VALUE|
				registry.ENUMERATE_SUB_KEYS|
				registry.WOW64_64KEY)
		if err != nil {
			continue
		}

		// Add to the LRU
		key_info, err = self.buildAndCacheKeyInfo(
			subkey, full_path.Append(subkey_name))
		if err == nil {
			result = append(result, key_info)
		}
		subkey.Close()
	}

	// Now enumerate the values.
	values, err := ReadValueNames(key)
	if err != nil {
		return nil, err
	}

	if len(values) > 0 {
		cached, ok := self.cache.Get(full_path.String())
		if !ok {
			cached, _ = self.buildAndCacheKeyInfo(key, full_path)
		}

		for _, value_name := range values {
			if value_name == "" {
				value_name = "@"
			}
			value_info, err := getValueInfo(
				cached.ModTime(),
				full_path.Append(value_name))
			if err != nil {
				continue
			}
			result = append(result, value_info)
		}
	}

	return result, nil
}

func (self RegFileSystemAccessor) OpenWithOSPath(path *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {

	metricsOpen.Inc()

	// Try to open the key as a value
	value_info, err := self.lstatValue(path)
	if err == nil {
		value_info.materialize()
		return NewValueBuffer(value_info._binary_data, value_info), nil
	}

	// Did we just open a key?
	stat, err := self.lstatKey(path)
	if err != nil {
		return nil, err
	}

	// Keys do not have any data so just include the Data as a json blob.
	serialized, _ := json.Marshal(stat.Data)
	return NewValueBuffer(serialized, stat), nil
}

func (self RegFileSystemAccessor) Open(path string) (
	accessors.ReadSeekCloser, error) {
	stat, err := self.Lstat(path)
	if err != nil {
		return nil, err
	}

	value_info, ok := stat.(*RegValueInfo)
	if ok {
		value_info.materialize()
		return NewValueBuffer(value_info._binary_data, stat), nil
	}

	// Keys do not have any data.
	return NewValueBuffer([]byte{}, stat), nil
}

func (self *RegFileSystemAccessor) Lstat(filename string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

// Try to get the path as a key.
func (self *RegFileSystemAccessor) lstatKey(
	full_path *accessors.OSPath) (*RegKeyInfo, error) {

	cached, ok := self.cache.Get(full_path.String())
	if ok {
		return cached, nil
	}

	hive, hive_key_path, err := getHiveAndKey(full_path)
	if err != nil {
		return nil, err
	}

	metricsOpenKey.Inc()
	key, err := registry.OpenKey(hive, hive_key_path,
		registry.READ|registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return nil, err
	}
	defer key.Close()

	// We opened the full_path as a key - cache it for next time.
	return self.buildAndCacheKeyInfo(key, full_path)
}

// Try to get the path as a key.
func (self *RegFileSystemAccessor) lstatValue(
	full_path *accessors.OSPath) (*RegValueInfo, error) {
	// Try to open it as a value
	if len(full_path.Components) == 0 {
		return nil, utils.NotFoundError
	}

	// Maybe its a value then - open the containing key
	// and return a valueInfo
	containing_key := full_path.Dirname()

	// We have the containing key in cache - use it.
	cached, ok := self.cache.Get(containing_key.String())
	if ok {
		return getValueInfo(cached.ModTime(), full_path)
	}

	// We need to open the containing key
	hive, hive_key_path, err := getHiveAndKey(containing_key)
	if err != nil {
		return nil, err
	}

	metricsOpenKey.Inc()
	key, err := registry.OpenKey(hive, hive_key_path,
		registry.READ|registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return nil, err
	}
	defer key.Close()

	cached, err = self.buildAndCacheKeyInfo(key, containing_key)
	if err != nil {
		return nil, err
	}
	return getValueInfo(cached.ModTime(), full_path)
}

func (self *RegFileSystemAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (accessors.FileInfo, error) {

	metricsStat.Inc()

	// Is the full path a key ?
	cached, ok := self.cache.Get(full_path.String())
	if ok {
		return cached, nil
	}

	// No: Try to open it as a key
	res, err := self.lstatKey(full_path)
	if err == nil {
		return res, nil
	}

	return self.lstatValue(full_path)
}

func (self *RegFileSystemAccessor) buildAndCacheKeyInfo(
	key registry.Key, full_path *accessors.OSPath) (
	*RegKeyInfo, error) {

	stat, err := key.Stat()
	if err != nil {
		return nil, err
	}

	res := &RegKeyInfo{
		_modtime:   stat.ModTime(),
		_full_path: full_path.Copy(),
		_data:      ordereddict.NewDict().Set("type", "key"),
	}

	cache_key := full_path.String()
	self.cache.Set(cache_key, res)
	return res, nil
}

func getValueInfo(
	key_modtime time.Time,
	full_path *accessors.OSPath) (*RegValueInfo, error) {

	return &RegValueInfo{
		RegKeyInfo: RegKeyInfo{
			// Values do not carry their own
			// timestamp - the key they are in
			// gets its timestamp updated whenever
			// any of the values does so we just
			// copy the key's timestamp to each
			// value.
			_modtime:   key_modtime,
			_full_path: full_path.Copy(),
			_data:      nil, // Not materialized yet - lazy
		}}, nil
}

func getHiveAndKey(full_path *accessors.OSPath) (registry.Key, string, error) {
	if len(full_path.Components) == 0 {
		return 0, "", errors.New("Invalid Path")
	}

	hive_name := full_path.Components[0]
	hive_any, pres := root_keys.Get(hive_name)
	if !pres {
		// Not a real hive
		return 0, "", errors.New("Unknown hive")
	}

	hive := hive_any.(registry.Key)

	// Produce a string to use on the OpenKey API - the key is joined
	// with \ on all components after the hive.
	key_path := ""
	if len(full_path.Components) > 1 {
		key_path = strings.Join(full_path.Components[1:], "\\")
	}

	return hive, key_path, nil
}

func init() {
	accessors.Register(&RegFileSystemAccessor{})
	json.RegisterCustomEncoder(&RegKeyInfo{}, accessors.MarshalGlobFileInfo)
	json.RegisterCustomEncoder(&RegValueInfo{}, accessors.MarshalGlobFileInfo)
}
