// A filesystem accessor for accessing the windows registry.

// We make the registry look like a filesystem:
// 1. Keys are mapped as directories, and values are files.
// 2. Map the root path to a virtual directory containing all the root keys.
// 3. Normalized paths contain / for directory separators.
// 4. Normalized paths have reg: prefix.
package glob

import (
	"encoding/json"
	"fmt"
	errors "github.com/pkg/errors"
	"golang.org/x/sys/windows/registry"
	"os"
	"regexp"
	"strings"
	"time"
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
	_modtime   time.Time
	_full_path string
	_name      string
	Type       string
	Data       string
}

func (self *RegKeyInfo) IsDir() bool {
	return true
}

func (self *RegKeyInfo) Size() int64 {
	return 0
}

func (self *RegKeyInfo) Sys() interface{} {
	return nil
}

func (self *RegKeyInfo) FullPath() string {
	return self._full_path
}

func (self *RegKeyInfo) Mode() os.FileMode {
	return 0755 | os.ModeDir
}

func (self *RegKeyInfo) Name() string {
	return self._name
}

func (self *RegKeyInfo) ModTime() time.Time {
	return self._modtime
}

func (self *RegKeyInfo) Mtime() TimeVal {
	nsec := self.ModTime().UnixNano()
	return TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *RegKeyInfo) Ctime() TimeVal {
	return self.Mtime()
}

func (self *RegKeyInfo) Atime() TimeVal {
	return self.Mtime()
}

func (self *RegKeyInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Type     string
		Mtime    TimeVal
		Ctime    TimeVal
		Atime    TimeVal
	}{
		FullPath: self.FullPath(),
		Mtime:    self.Mtime(),
		Ctime:    self.Ctime(),
		Atime:    self.Atime(),
		Type:     "key",
	})

	return result, err
}

func (u *RegKeyInfo) UnmarshalJSON(data []byte) error {
	return nil
}

type RegValueInfo struct {
	RegKeyInfo
	Data interface{}
	Type string
}

func (self *RegValueInfo) Sys() interface{} {
	return self.Data
}

func (self *RegValueInfo) IsDir() bool {
	return true
}

func (self *RegValueInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Type     string
		Data     interface{}
		Mtime    TimeVal
		Ctime    TimeVal
		Atime    TimeVal
	}{
		FullPath: self.FullPath(),
		Mtime:    self.Mtime(),
		Ctime:    self.Ctime(),
		Atime:    self.Atime(),
		Type:     self.Type,
		Data:     self.Data,
	})

	return result, err
}

type RegFileSystemAccessor struct{}

func (self RegFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	var result []FileInfo
	path = normalize_path(path)

	if path == "\\" {
		for k, _ := range root_keys {
			result = append(result, &DrivePath{k})
		}

		return result, nil
	}

	// Add a final \ to turn path into a directory path.
	path = strings.TrimPrefix(path, "\\")
	path = strings.TrimSuffix(path, "\\")
	components := strings.Split(path, "\\")

	hive, pres := root_keys[components[0]]
	if !pres {
		// Not a real hive
		return nil, errors.New("Unknown hive")
	}

	key_path := ""
	// e.g. HKEY_PERFORMANCE_DATA
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

		stat, err := subkey.Stat()
		if err != nil {
			continue
		}

		result = append(result, &RegKeyInfo{
			_name:      subkey_name,
			_modtime:   stat.ModTime(),
			_full_path: "\\" + path + "\\" + subkey_name,
		})
	}

	// Now enumerate the values.
	values, err := key.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}

	for _, value_name := range values {
		value_info := &RegValueInfo{RegKeyInfo{
			_full_path: "\\" + path + "\\" + value_name,
			_name:      value_name,
		}, nil, ""}

		buf_size, value_type, err := key.GetValue(value_name, nil)
		if err != nil {
			continue
		}

		switch value_type {
		case registry.DWORD, registry.DWORD_BIG_ENDIAN, registry.QWORD:
			data, _, err := key.GetIntegerValue(value_name)
			if err != nil {
				continue
			}

			switch value_type {
			case registry.DWORD_BIG_ENDIAN:
				value_info.Type = "DWORD_BIG_ENDIAN"
			case registry.DWORD:
				value_info.Type = "DWORD"
			case registry.QWORD:
				value_info.Type = "QWORD"
			}

			value_info.Data = data

		case registry.BINARY:
			if buf_size < MAX_EMBEDDED_REG_VALUE {
				data, _, err := key.GetBinaryValue(value_name)
				if err != nil {
					continue
				}

				value_info.Data = data
			}
			value_info.Type = "BINARY"

		case registry.MULTI_SZ:
			if buf_size < MAX_EMBEDDED_REG_VALUE {
				values, _, err := key.GetStringsValue(value_name)
				if err != nil {
					continue
				}

				value_info.Data = values
			}
			value_info.Type = "MULTI_SZ"

		case registry.SZ, registry.EXPAND_SZ:
			switch value_type {
			case registry.SZ:
				value_info.Type = "SZ"
			case registry.EXPAND_SZ:
				value_info.Type = "EXPAND_SZ"
			}

			if buf_size < MAX_EMBEDDED_REG_VALUE {
				data, _, err := key.GetStringValue(value_name)
				if err != nil {
					continue
				}

				// We do not expand the key because
				// this will depend on the agent's own
				// environment strings.
				value_info.Data = data
			}

		default:
			value_info.Type = fmt.Sprintf("%d", value_type)
			if buf_size < MAX_EMBEDDED_REG_VALUE {
				buf := make([]byte, buf_size)
				_, _, err := key.GetValue(value_name, buf)
				if err != nil {
					continue
				}

				value_info.Data = buf
			}
		}

		result = append(result, value_info)
	}
	return result, nil
}

func (self RegFileSystemAccessor) GetRoot(path string) string {
	return "/"
}

func (self RegFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	path = strings.TrimPrefix(normalize_path(path), "\\")
	// Strip leading \\ so \\c:\\windows -> c:\\windows
	file, err := os.Open(path)
	return file, err
}

func (self *RegFileSystemAccessor) Lstat(filename string) (FileInfo, error) {
	return &DrivePath{"\\"}, nil
}

// We accept both / and \ as a path separator
func (self *RegFileSystemAccessor) PathSep() *regexp.Regexp {
	return regexp.MustCompile("[\\\\/]")
}

func init() {
	Register("reg", &RegFileSystemAccessor{})
}
