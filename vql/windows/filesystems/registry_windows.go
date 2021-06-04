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
package filesystems

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
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
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

type RegKeyInfo struct {
	_modtime    time.Time
	_components []string
	_data       *ordereddict.Dict
}

func (self *RegKeyInfo) IsDir() bool {
	return true
}

func (self *RegKeyInfo) Data() interface{} {
	return self._data
}

func (self *RegKeyInfo) Size() int64 {
	return 0
}

func (self *RegKeyInfo) Sys() interface{} {
	return nil
}

func (self *RegKeyInfo) FullPath() string {
	return utils.JoinComponents(self._components, "\\")
}

func (self *RegKeyInfo) Mode() os.FileMode {
	return 0755 | os.ModeDir
}

func (self *RegKeyInfo) Name() string {
	if len(self._components) > 0 {
		return self._components[len(self._components)-1]
	}
	return ""
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

func (self *RegKeyInfo) GetLink() (string, error) {
	return "", errors.New("Not implemented")
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

func (self *RegValueInfo) Sys() interface{} {
	return self._data
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
	info glob.FileInfo
}

func (self *ValueBuffer) Stat() (os.FileInfo, error) {
	return self.info, nil
}

func (self *ValueBuffer) Close() error {
	return nil
}

func NewValueBuffer(buf []byte, stat glob.FileInfo) *ValueBuffer {
	return &ValueBuffer{
		bytes.NewReader(buf),
		stat,
	}
}

type RegFileSystemAccessor struct{}

func (self *RegFileSystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	return self, nil
}

func (self RegFileSystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	var result []glob.FileInfo

	components := utils.SplitComponents(path)

	// Root directory is just the name of the hives.
	if len(components) == 0 {
		for k, _ := range root_keys {
			result = append(result,
				glob.NewVirtualDirectoryPath(k, nil))
		}
		return result, nil
	}

	hive_name := components[0]
	hive, pres := root_keys[hive_name]
	if !pres {
		// Not a real hive
		return nil, errors.New("Unknown hive")
	}

	key_path := ""
	// e.g. HKEY_PERFORMANCE_DATA
	// Add a final \ to turn path into a directory path.
	if len(components) > 1 {
		key_path = strings.Join(components[1:], "\\")
	}

	key, err := registry.OpenKey(hive, key_path,
		registry.READ|
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
			registry.READ|
				registry.ENUMERATE_SUB_KEYS|
				registry.WOW64_64KEY)
		if err != nil {
			continue
		}
		defer subkey.Close()

		// Make a local copy.
		full_path := append([]string{}, components...)
		key_info, err := getKeyInfo(subkey, append(full_path, subkey_name))
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
		// Make a local copy.
		full_path := append([]string{}, components...)

		value_info, err := getValueInfo(key,
			append(full_path, value_name))
		if err != nil {
			continue
		}
		result = append(result, value_info)
	}

	return result, nil
}

func (self RegFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
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

func (self *RegFileSystemAccessor) Lstat(filename string) (glob.FileInfo, error) {
	components := utils.SplitComponents(filename)
	if len(components) == 0 {
		return nil, errors.New("No filename given")
	}

	hive_name := components[0]
	hive, pres := root_keys[hive_name]
	if !pres {
		// Not a real hive
		return nil, errors.New("Unknown hive")
	}

	key_path := ""
	// e.g. HKEY_PERFORMANCE_DATA
	// Add a final \ to turn path into a directory path.
	if len(components) > 1 {
		key_path = strings.Join(components[1:], "\\")
	}

	key, err := registry.OpenKey(hive, key_path,
		registry.READ|registry.WOW64_64KEY)
	if err != nil && len(components) > 1 {
		// Maybe its a value then - open the containing key
		// and return a valueInfo
		containing_key_name := strings.Join(
			components[1:len(components)-1], "\\")
		key, err := registry.OpenKey(hive, containing_key_name,
			registry.READ|registry.WOW64_64KEY)
		if err != nil {
			return nil, err
		}
		defer key.Close()

		return getValueInfo(key, components)
	}
	defer key.Close()

	return getKeyInfo(key, components)
}

func getKeyInfo(key registry.Key, components []string) (*RegKeyInfo, error) {
	stat, err := key.Stat()
	if err != nil {
		return nil, err
	}
	return &RegKeyInfo{
		_modtime:    stat.ModTime(),
		_components: components,
		_data:       ordereddict.NewDict().Set("type", "key"),
	}, nil
}

func getValueInfo(key registry.Key, components []string) (*RegValueInfo, error) {
	// Last component is the value name
	value_name := components[len(components)-1]

	// Represent the default value as different from the
	// actual key name.
	if value_name == "" {
		components[len(components)-1] = "@"
	}

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
			_modtime:    key_modtime,
			_components: components,
		}}

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

func (self RegFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "", path, nil
}

// We accept both / and \ as a path separator
func (self RegFileSystemAccessor) PathSplit(path string) []string {
	return utils.SplitComponents(path)
}

func (self RegFileSystemAccessor) PathJoin(root, stem string) string {
	return utils.PathJoin(root, stem, "\\")
}

func init() {
	glob.Register("reg", &RegFileSystemAccessor{})
	glob.Register("registry", &RegFileSystemAccessor{})

	json.RegisterCustomEncoder(&RegKeyInfo{}, glob.MarshalGlobFileInfo)
	json.RegisterCustomEncoder(&RegValueInfo{}, glob.MarshalGlobFileInfo)
}
