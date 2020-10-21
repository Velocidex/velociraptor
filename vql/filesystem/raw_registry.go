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
package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/regparser"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	MAX_EMBEDDED_REG_VALUE = 4 * 1024
)

type RawRegKeyInfo struct {
	key         *regparser.CM_KEY_NODE
	_base       url.URL
	_components []string
}

func (self *RawRegKeyInfo) IsDir() bool {
	return true
}

func (self *RawRegKeyInfo) Data() interface{} {
	return ordereddict.NewDict().Set("type", "Key")
}

func (self *RawRegKeyInfo) Size() int64 {
	return 0
}

func (self *RawRegKeyInfo) Sys() interface{} {
	return nil
}

func (self *RawRegKeyInfo) FullPath() string {
	self._base.Fragment = utils.JoinComponents(self._components, "\\")
	return self._base.String()
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

func (self *RawRegKeyInfo) Mtime() utils.TimeVal {
	nsec := self.ModTime().UnixNano()
	return utils.TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *RawRegKeyInfo) Ctime() utils.TimeVal {
	return self.Mtime()
}

func (self *RawRegKeyInfo) Atime() utils.TimeVal {
	return self.Mtime()
}

// Not supported
func (self *RawRegKeyInfo) IsLink() bool {
	return false
}

func (self *RawRegKeyInfo) GetLink() (string, error) {
	return "", errors.New("Not implemented")
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

func (self *RawRegValueInfo) Sys() interface{} {
	return self.value.ValueData()
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

func (self *RawRegValueInfo) Data() interface{} {
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

func (self *RawValueBuffer) Stat() (os.FileInfo, error) {
	return self.info, nil
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

type RawRegistryFileCache struct {
	registry *regparser.Registry
	fd       glob.ReadSeekCloser
}

type RawRegFileSystemAccessor struct {
	mu       sync.Mutex
	fd_cache map[string]*RawRegistryFileCache
	scope    *vfilter.Scope
}

func (self *RawRegFileSystemAccessor) getRegHive(
	file_path string) (*RawRegistryFileCache, *url.URL, error) {
	url, err := url.Parse(file_path)
	if err != nil {
		return nil, nil, err
	}

	base_url := *url
	base_url.Fragment = ""

	self.mu.Lock()
	defer self.mu.Unlock()

	file_cache, pres := self.fd_cache[base_url.String()]
	if !pres {
		accessor, err := glob.GetAccessor(url.Scheme, self.scope)
		if err != nil {
			return nil, nil, err
		}

		fd, err := accessor.Open(url.Path)
		if err != nil {
			return nil, nil, err
		}

		reader, ok := fd.(io.ReaderAt)
		if !ok {
			return nil, nil, errors.New("file is not seekable")
		}

		registry, err := regparser.NewRegistry(reader)
		if err != nil {
			return nil, nil, err
		}

		file_cache = &RawRegistryFileCache{
			registry: registry,
			fd:       fd,
		}

		self.fd_cache[url.String()] = file_cache
	}

	return file_cache, url, nil
}

const RawRegFileSystemTag = "_RawReg"

func (self *RawRegFileSystemAccessor) New(scope *vfilter.Scope) (
	glob.FileSystemAccessor, error) {

	result_any := vql_subsystem.CacheGet(scope, RawRegFileSystemTag)
	if result_any == nil {
		result := &RawRegFileSystemAccessor{
			fd_cache: make(map[string]*RawRegistryFileCache),
			scope:    scope,
		}
		vql_subsystem.CacheSet(scope, RawRegFileSystemTag, result)

		// When scope is destroyed, we close all the filehandles.
		scope.AddDestructor(func() {
			result.mu.Lock()
			defer result.mu.Unlock()

			for _, v := range result.fd_cache {
				v.fd.Close()
			}
		})
		return result, nil
	}

	return result_any.(glob.FileSystemAccessor), nil
}

func (self *RawRegFileSystemAccessor) ReadDir(key_path string) ([]glob.FileInfo, error) {
	var result []glob.FileInfo
	file_cache, url, err := self.getRegHive(key_path)
	if err != nil {
		return nil, err
	}
	key := file_cache.registry.OpenKey(url.Fragment)
	if key == nil {
		return nil, errors.New("Key not found")
	}

	components := utils.SplitComponents(url.Fragment)

	for _, subkey := range key.Subkeys() {
		new_components := append([]string{}, components...)
		result = append(result,
			&RawRegKeyInfo{
				key:         subkey,
				_base:       *url,
				_components: append(new_components, subkey.Name()),
			})
	}

	for _, value := range key.Values() {
		new_components := append([]string{}, components...)

		result = append(result,
			&RawRegValueInfo{
				&RawRegKeyInfo{
					key:         key,
					_base:       *url,
					_components: append(new_components, value.ValueName()),
				}, value,
			})
	}

	return result, nil
}

func (self *RawRegFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	return nil, errors.New("Not implemented")
}

func (self *RawRegFileSystemAccessor) Lstat(filename string) (glob.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self *RawRegFileSystemAccessor) GetRoot(path string) (string, string, error) {
	url, err := url.Parse(path)
	if err != nil {
		return "", "", err
	}

	fragment := url.Fragment
	url.Fragment = ""

	return url.String() + "#", fragment, nil
}

// We accept both / and \ as a path separator
func (self *RawRegFileSystemAccessor) PathSplit(path string) []string {
	return utils.SplitComponents(path)
}

func (self *RawRegFileSystemAccessor) PathJoin(root, stem string) string {
	url, err := url.Parse(root)
	if err != nil {
		fmt.Printf("Error %v Joining %v and %v -> %v\n",
			err, root, stem, path.Join(root, stem))
		return path.Join(root, stem)
	}

	url.Fragment = utils.PathJoin(url.Fragment, stem, "/")

	result := url.String()

	return result
}

type ReadKeyValuesArgs struct {
	Globs    []string `vfilter:"required,field=globs,doc=Glob expressions to apply."`
	Accessor string   `vfilter:"optional,field=accessor,default=reg,doc=The accessor to use."`
}

type ReadKeyValues struct{}

func (self ReadKeyValues) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	globber := make(glob.Globber)
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		config_obj, ok := artifacts.GetServerConfig(scope)
		if !ok {
			config_obj = &config_proto.Config{}
		}

		arg := &ReadKeyValuesArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("read_reg_key: %s", err.Error())
			return
		}

		accessor_name := arg.Accessor
		if accessor_name == "" {
			accessor_name = "reg"
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("read_reg_key: %s", err.Error())
			return
		}

		accessor, err := glob.GetAccessor(accessor_name, scope)
		if err != nil {
			scope.Log("read_reg_key: %v", err)
			return
		}
		root := ""
		for _, item := range arg.Globs {
			item_root, item_path, _ := accessor.GetRoot(item)
			if root != "" && root != item_root {
				scope.Log("glob: %s: Must use the same root for "+
					"all globs. Skipping.", item)
				continue
			}
			root = item_root
			err = globber.Add(item_path, accessor.PathSplit)
			if err != nil {
				scope.Log("glob: %v", err)
				return
			}
		}

		file_chan := globber.ExpandWithContext(
			ctx, config_obj, root, accessor)
		for {
			select {
			case <-ctx.Done():
				return

			case f, ok := <-file_chan:
				if !ok {
					return
				}
				if f.IsDir() {
					res := ordereddict.NewDict().
						SetDefault(&vfilter.Null{}).
						SetCaseInsensitive().
						Set("Key", f)
					values, err := accessor.ReadDir(f.FullPath())
					if err != nil {
						continue
					}

					for _, item := range values {
						value_info, ok := item.(glob.FileInfo)
						if ok {
							value_data, ok := value_info.Data().(*ordereddict.Dict)
							if ok && value_data != nil {
								value, pres := value_data.Get("value")
								if pres {
									res.Set(item.Name(), value)
								}
							}
						}
					}

					select {
					case <-ctx.Done():
						return

					case output_chan <- res:
					}
				}
			}
		}
	}()

	return output_chan
}

func (self ReadKeyValues) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "read_reg_key",
		Doc: "This is a convenience function for reading the entire " +
			"registry key matching the globs. Emits dicts with keys " +
			"being the value names and the values being the value data.",
		ArgType: type_map.AddType(scope, &ReadKeyValuesArgs{}),
	}
}

func init() {
	glob.Register("raw_reg", &RawRegFileSystemAccessor{})
	vql_subsystem.RegisterPlugin(&ReadKeyValues{})

	json.RegisterCustomEncoder(&RawRegKeyInfo{}, glob.MarshalGlobFileInfo)
	json.RegisterCustomEncoder(&RawRegValueInfo{}, glob.MarshalGlobFileInfo)
}
