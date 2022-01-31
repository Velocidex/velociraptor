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
// A filesystem accessor for accessing registry hives through raw
// file parsing.

// We make the registry look like a filesystem:
// 1. Keys are mapped as directories, and values are files.
// 2. The file is interpreted as a URL with the following format:
//    accessor:/path#key_path
// 3. We use the accessor and path to open the underlying file, then
//    extract the key or value named by the key_path from it.
// 4. Normalized paths contain / for directory separators.
// 5. Normalized paths have rawreg: prefix.
package raw_registry

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
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
}

func (self *RawRegValueInfo) Name() string {
	return self.value.ValueName()
}

func (self *RawRegValueInfo) IsDir() bool {
	return false
}

func (self *RawRegValueInfo) Mode() os.FileMode {
	return 0755
}

func (self *RawRegValueInfo) Size() int64 {
	return int64(self.value.DataSize())
}

func (self *RawRegValueInfo) Data() *ordereddict.Dict {
	value_data := self.value.ValueData()
	result := ordereddict.NewDict().
		Set("type", self.value.TypeString()).
		Set("data_len", len(value_data.Data))

	switch value_data.Type {
	case regparser.REG_SZ, regparser.REG_MULTI_SZ, regparser.REG_EXPAND_SZ:
		result.Set("value", strings.TrimRight(value_data.String, "\x00"))

	case regparser.REG_DWORD, regparser.REG_QWORD, regparser.REG_DWORD_BIG_ENDIAN:
		result.Set("value", value_data.Uint64)
	default:
		if len(value_data.Data) < MAX_EMBEDDED_REG_VALUE {
			result.Set("value", value_data.Data)
		}
	}
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

type RawRegFileSystemAccessor struct {
	mu sync.Mutex

	// Maintain a cache of already parsed hives
	hive_cache map[string]*regparser.Registry
	scope      vfilter.Scope
}

func (self *RawRegFileSystemAccessor) getRegHive(
	file_path *accessors.OSPath) (*regparser.Registry, error) {

	// Cache the parsed hive under the underlying file.
	pathspec := file_path.PathSpec()
	base_pathspec := accessors.PathSpec{
		DelegateAccessor: pathspec.DelegateAccessor,
		DelegatePath:     pathspec.GetDelegatePath(),
	}
	cache_key := base_pathspec.String()

	self.mu.Lock()
	defer self.mu.Unlock()

	lru_size := vql_subsystem.GetIntFromRow(
		self.scope, self.scope, constants.RAW_REG_CACHE_SIZE)
	hive, pres := self.hive_cache[cache_key]
	if !pres {
		paged_reader, err := readers.NewPagedReader(
			self.scope,
			pathspec.DelegateAccessor,
			pathspec.GetDelegatePath(),
			int(lru_size),
		)
		if err != nil {
			self.scope.Log("%v: did you provide a URL or Pathspec?", err)
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

		hive, err = regparser.NewRegistry(paged_reader)
		if err != nil {
			paged_reader.Close()
			return nil, err
		}

		self.hive_cache[cache_key] = hive
	}

	return hive, nil
}

const RawRegFileSystemTag = "_RawReg"

func (self *RawRegFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	result_any := vql_subsystem.CacheGet(scope, RawRegFileSystemTag)
	if result_any == nil {
		result := &RawRegFileSystemAccessor{
			hive_cache: make(map[string]*regparser.Registry),
			scope:      scope,
		}
		vql_subsystem.CacheSet(scope, RawRegFileSystemTag, result)
		return result, nil
	}

	return result_any.(accessors.FileSystemAccessor), nil
}

func (self *RawRegFileSystemAccessor) ReadDir(key_path string) (
	[]accessors.FileInfo, error) {

	full_path := accessors.NewWindowsOSPath(key_path)

	var result []accessors.FileInfo
	hive, err := self.getRegHive(full_path)
	if err != nil {
		return nil, err
	}

	key := OpenKeyComponents(hive, full_path.Components)
	if key == nil {
		return nil, errors.New("Key not found")
	}

	for _, subkey := range key.Subkeys() {
		result = append(result,
			&RawRegKeyInfo{
				key:        subkey,
				_full_path: full_path.Append(subkey.Name()),
			})
	}

	for _, value := range key.Values() {
		result = append(result,
			&RawRegValueInfo{
				&RawRegKeyInfo{
					key:        key,
					_full_path: full_path.Append(value.ValueName()),
				}, value,
			})
	}

	return result, nil
}

func (self *RawRegFileSystemAccessor) Open(path string) (
	accessors.ReadSeekCloser, error) {
	return nil, errors.New("Not implemented")
}

func (self *RawRegFileSystemAccessor) Lstat(filename string) (
	accessors.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func init() {
	accessors.Register("raw_reg", &RawRegFileSystemAccessor{},
		`Access keys and values by parsing the raw registry hive. Path is a pathspec having delegate opening the raw registry hive.`)

	json.RegisterCustomEncoder(&RawRegKeyInfo{}, accessors.MarshalGlobFileInfo)
	json.RegisterCustomEncoder(&RawRegValueInfo{}, accessors.MarshalGlobFileInfo)
}
