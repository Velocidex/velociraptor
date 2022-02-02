// +build windows

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
// A filesystem accessor for accessing the windows registry.

// We make the registry look like a filesystem:
// 1. Keys are mapped as directories, and values are files.
// 2. Map the root path to a virtual directory containing all the root keys.
// 3. Normalized paths contain / for directory separators.
// 4. Normalized paths have reg: prefix.
package registry

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"golang.org/x/sys/windows/registry"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

var (
	// Include some common aliases.
	root_keys = map[string]registry.Key{
		"HKEY_CLASSES_ROOT":     registry.CLASSES_ROOT,
		"HKEY_CURRENT_USER":     registry.CURRENT_USER,
		"HKEY_LOCAL_MACHINE":    registry.LOCAL_MACHINE,
		"HKEY_USERS":            registry.USERS,
		"HKEY_CURRENT_CONFIG":   registry.CURRENT_CONFIG,
		"HKEY_PERFORMANCE_DATA": registry.PERFORMANCE_DATA,
	}

	// Values smaller than this will be included in the stat entry
	// itself.
	MAX_EMBEDDED_REG_VALUE = 4 * 1024
)

func GetHiveFromName(name string) (registry.Key, bool) {
	hive, pres := root_keys[name]
	return hive, pres
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

func (self *RegKeyInfo) FullPath() string {
	return self._full_path.String()
}

func (self *RegKeyInfo) OSPath() *accessors.OSPath {
	return self._full_path
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

func (self *RegValueInfo) Mode() os.FileMode {
	return 0755
}

func (self *RegValueInfo) Size() int64 {
	return self._size
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

type RegFileSystemAccessor struct{}

func (self *RegFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	return self, nil
}

func (self RegFileSystemAccessor) ParsePath(path string) *accessors.OSPath {
	return accessors.NewWindowsRegistryPath(path)
}

func (self RegFileSystemAccessor) ReadDir(path string) (
	[]accessors.FileInfo, error) {
	var result []accessors.FileInfo

	full_path := self.ParsePath(path)
	path = full_path.PathSpec().Path

	// Root directory is just the name of the hives.
	if len(full_path.Components) == 0 {
		for k, _ := range root_keys {
			result = append(result, &accessors.VirtualFileInfo{
				IsDir_: true,
				Path:   full_path.Append(k),
				Data_: ordereddict.NewDict().
					Set("type", "hive"),
			})
		}
		return result, nil
	}

	hive_name := full_path.Components[0]
	hive, pres := root_keys[hive_name]
	if !pres {
		// Not a real hive
		return nil, errors.New("Unknown hive")
	}

	key_path := ""
	// e.g. HKEY_PERFORMANCE_DATA
	// Add a final \ to turn path into a directory path.
	if len(full_path.Components) > 1 {
		key_path = strings.Join(full_path.Components[1:], "\\")
	}

	key, err := registry.OpenKey(hive, key_path,
		registry.READ|registry.QUERY_VALUE|
			registry.ENUMERATE_SUB_KEYS|
			registry.WOW64_64KEY)
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
		subkey, err := registry.OpenKey(key, subkey_name,
			registry.READ|registry.QUERY_VALUE|
				registry.ENUMERATE_SUB_KEYS|
				registry.WOW64_64KEY)
		if err != nil {
			continue
		}
		defer subkey.Close()

		// Make a local copy.
		key_info, err := getKeyInfo(subkey, full_path.Append(subkey_name))
		if err != nil {
			continue
		}
		result = append(result, key_info)
	}

	// Now enumerate the values.
	values, err := key.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}

	for _, value_name := range values {
		if value_name == "" {
			value_name = "@"
		}
		value_info, err := getValueInfo(
			key, full_path.Append(value_name))
		if err != nil {
			continue
		}
		result = append(result, value_info)
	}

	return result, nil
}

func (self RegFileSystemAccessor) Open(path string) (
	accessors.ReadSeekCloser, error) {
	stat, err := self.Lstat(path)
	if err != nil {
		return nil, err
	}

	value_info, ok := stat.(*RegValueInfo)
	if ok {
		return NewValueBuffer(value_info._binary_data, stat), nil
	}

	// Keys do not have any data.
	return NewValueBuffer([]byte{}, stat), nil
}

func (self *RegFileSystemAccessor) Lstat(filename string) (
	accessors.FileInfo, error) {

	// Clean the path
	full_path := accessors.NewWindowsRegistryPath(filename)
	if len(full_path.Components) == 0 {
		return nil, errors.New("No filename given")
	}

	hive_name := full_path.Components[0]
	hive, pres := root_keys[hive_name]
	if !pres {
		// Not a real hive
		return nil, errors.New("Unknown hive")
	}

	// The key path inside the hive
	key_path := full_path.TrimComponents(hive_name)

	hive_key_path := ""
	// Convert the path into an OS specific string
	if len(full_path.Components) > 1 {
		hive_key_path = strings.Join(full_path.Components, "\\")
	}

	key, err := registry.OpenKey(hive, hive_key_path,
		registry.READ|registry.QUERY_VALUE|
			registry.WOW64_64KEY)
	if err != nil && len(key_path.Components) > 0 {

		// Maybe its a value then - open the containing key
		// and return a valueInfo
		containing_key := key_path.Dirname()
		containing_key_name := strings.Join(containing_key.Components, "\\")
		key, err := registry.OpenKey(hive, containing_key_name,
			registry.READ|registry.QUERY_VALUE|
				registry.WOW64_64KEY)
		if err != nil {
			return nil, err
		}
		defer key.Close()

		return getValueInfo(key, full_path)
	}
	defer key.Close()

	return getKeyInfo(key, full_path)
}

func getKeyInfo(key registry.Key, full_path *accessors.OSPath) (
	*RegKeyInfo, error) {

	stat, err := key.Stat()
	if err != nil {
		return nil, err
	}
	return &RegKeyInfo{
		_modtime:   stat.ModTime(),
		_full_path: full_path,
		_data:      ordereddict.NewDict().Set("type", "key"),
	}, nil
}

func getValueInfo(key registry.Key, full_path *accessors.OSPath) (
	*RegValueInfo, error) {

	// Last component is the value name
	value_name := full_path.Basename()

	var key_modtime time.Time
	key_stat, err := key.Stat()
	if err == nil {
		key_modtime = key_stat.ModTime()
	}

	value_info := &RegValueInfo{
		RegKeyInfo: RegKeyInfo{
			// Values do not carry their own
			// timestamp - the key they are in
			// gets its timestamp updated whenever
			// any of the values does so we just
			// copy the key's timestamp to each
			// value.
			_modtime:   key_modtime,
			_full_path: full_path,
		}}

	// Internally we represent the default value of a key as the name
	// '@'
	if value_name == "@" {
		value_name = ""
	}

	buf_size, value_type, err := key.GetValue(value_name, nil)
	if err != nil {
		return nil, err
	}

	value_info._size = int64(buf_size)

	switch value_type {
	case registry.DWORD, registry.DWORD_BIG_ENDIAN, registry.QWORD:
		data, _, err := key.GetIntegerValue(value_name)
		if err != nil {
			return nil, err
		}

		switch value_type {
		case registry.DWORD_BIG_ENDIAN:
			value_info.Type = "DWORD_BIG_ENDIAN"
		case registry.DWORD:
			value_info.Type = "DWORD"
		case registry.QWORD:
			value_info.Type = "QWORD"
		}

		value_info._data = ordereddict.NewDict().
			Set("type", value_info.Type).
			Set("value", data)

	case registry.BINARY:
		data, _, err := key.GetBinaryValue(value_name)
		if err != nil {
			return nil, err
		}

		if buf_size < MAX_EMBEDDED_REG_VALUE {
			value_info._data = ordereddict.NewDict().
				Set("type", "BINARY").
				Set("value", data)
		}
		value_info._binary_data = data
		value_info.Type = "BINARY"

	case registry.MULTI_SZ:
		values, _, err := key.GetStringsValue(value_name)
		if err != nil {
			return nil, err
		}
		value_info._binary_data = []byte(strings.Join(values, "\n"))
		value_info.Type = "MULTI_SZ"

		if buf_size < MAX_EMBEDDED_REG_VALUE {
			value_info._data = ordereddict.NewDict().
				Set("type", "MULTI_SZ").
				Set("value", values)
		}

	case registry.SZ, registry.EXPAND_SZ:
		switch value_type {
		case registry.SZ:
			value_info.Type = "SZ"
		case registry.EXPAND_SZ:
			value_info.Type = "EXPAND_SZ"
		}

		data, _, err := key.GetStringValue(value_name)
		if err != nil {
			return nil, err
		}
		value_info._binary_data = []byte(data)

		if buf_size < MAX_EMBEDDED_REG_VALUE {
			value_info._data = ordereddict.NewDict().
				Set("type", value_info.Type).
				// We do not expand the value data
				// because this will depend on the
				// agent's own environment strings.
				Set("value", data)
		}

	default:
		buf := make([]byte, buf_size)
		_, _, err := key.GetValue(value_name, buf)
		if err != nil {
			return nil, err
		}
		value_info._binary_data = buf
		value_info.Type = fmt.Sprintf("%d", value_type)

		if buf_size < MAX_EMBEDDED_REG_VALUE {
			value_info._data = ordereddict.NewDict().
				Set("type", value_info.Type).
				Set("value", buf)
		}
	}
	return value_info, nil
}

func init() {
	description := `Access the registery like a filesystem using the OS APIs.`
	accessors.Register("reg", &RegFileSystemAccessor{}, description)
	accessors.Register("registry", &RegFileSystemAccessor{}, description)

	json.RegisterCustomEncoder(&RegKeyInfo{}, accessors.MarshalGlobFileInfo)
	json.RegisterCustomEncoder(&RegValueInfo{}, accessors.MarshalGlobFileInfo)
}
