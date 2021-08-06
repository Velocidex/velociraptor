package memory

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

var (
	// Only used for tests.
	Test_memory_file_store *MemoryFileStore
)

func NewMemoryFileStore(config_obj *config_proto.Config) *MemoryFileStore {
	mu.Lock()
	defer mu.Unlock()

	if Test_memory_file_store == nil {
		Test_memory_file_store = &MemoryFileStore{
			Data:       ordereddict.NewDict(),
			config_obj: config_obj,
		}
	}

	// Sanitize the FilestoreDirectory
	config_obj.Datastore.FilestoreDirectory = strings.TrimSuffix(
		config_obj.Datastore.FilestoreDirectory, "/")

	return Test_memory_file_store
}

type MemoryReader struct {
	*bytes.Reader
	pathSpec_ api.FSPathSpec
	filename  string
}

func (self MemoryReader) Close() error {
	return nil
}

func (self MemoryReader) Stat() (api.FileInfo, error) {
	return vtesting.MockFileInfo{
		Name_:     self.filename,
		PathSpec_: self.pathSpec_,
		FullPath_: self.filename,
		Size_:     int64(self.Reader.Len()),
	}, nil
}

type MemoryWriter struct {
	buf               []byte
	memory_file_store *MemoryFileStore
	filename          string
}

func (self *MemoryWriter) Size() (int64, error) {
	return int64(len(self.buf)), nil
}

func (self *MemoryWriter) Write(data []byte) (int, error) {
	self.buf = append(self.buf, data...)
	return len(data), nil
}

func (self *MemoryWriter) Close() error {
	self.memory_file_store.mu.Lock()
	defer self.memory_file_store.mu.Unlock()

	self.memory_file_store.Data.Set(self.filename, self.buf)
	return nil
}

func (self *MemoryWriter) Truncate() error {
	self.buf = nil
	return nil
}

type MemoryFileStore struct {
	mu sync.Mutex

	config_obj *config_proto.Config
	Data       *ordereddict.Dict
}

func (self *MemoryFileStore) Debug() {
	self.mu.Lock()
	defer self.mu.Unlock()

	fmt.Printf("MemoryFileStore: \n")
	for _, k := range self.Data.Keys() {
		v_any, _ := self.Data.Get(k)
		v := v_any.([]byte)
		// Render index files especially
		if strings.HasSuffix(k, ".index") ||
			strings.HasSuffix(k, ".idx") ||
			strings.HasSuffix(k, ".tidx") {
			fmt.Printf("%v: %v\n", k, hex.Dump(v))
			continue
		}

		fmt.Printf("%v: %v\n", k, string(v))
	}
}

func (self *MemoryFileStore) ReadFile(path api.FSPathSpec) (api.FileReader, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	filename := pathSpecToPath(path, self.config_obj)
	self.Trace("ReadFile", filename)
	data_any, pres := self.Data.Get(filename)
	if pres {
		data := data_any.([]byte)
		return MemoryReader{
			Reader:    bytes.NewReader(data),
			pathSpec_: path,
			filename:  filename,
		}, nil
	}

	return nil, errors.New("Not found")
}

func (self *MemoryFileStore) WriteFile(path api.FSPathSpec) (api.FileWriter, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	filename := pathSpecToPath(path, self.config_obj)
	self.Trace("WriteFile", filename)
	buf, pres := self.Data.Get(filename)
	if !pres {
		buf = []byte{}
	}
	self.Data.Set(filename, buf)

	return &MemoryWriter{
		buf:               buf.([]byte),
		memory_file_store: self,
		filename:          filename,
	}, nil
}

func (self *MemoryFileStore) StatFile(path api.FSPathSpec) (api.FileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	filename := pathSpecToPath(path, self.config_obj)
	self.Trace("StatFile", filename)
	buff, pres := self.Data.Get(filename)
	if !pres {
		return nil, os.ErrNotExist
	}

	return &vtesting.MockFileInfo{
		Name_:     path.Base(),
		FullPath_: filename,
		Size_:     int64(len(buff.([]byte))),
	}, nil
}

func (self *MemoryFileStore) Move(src, dest api.FSPathSpec) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	src_filename := pathSpecToPath(src, self.config_obj)
	dest_filename := pathSpecToPath(dest, self.config_obj)
	buff, pres := self.Data.Get(src_filename)
	if !pres {
		return os.ErrNotExist
	}

	self.Data.Set(dest_filename, buff)
	self.Data.Delete(src_filename)
	return nil
}

func (self *MemoryFileStore) ListDirectory(path api.FSPathSpec) ([]api.FileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	dirname := pathDirSpecToPath(path, self.config_obj)
	self.Trace("ListDirectory", dirname)
	result := []api.FileInfo{}
	files := []string{}
	for _, filename := range self.Data.Keys() {
		v_any, _ := self.Data.Get(filename)
		v := v_any.([]byte)

		if strings.HasPrefix(filename, dirname) {
			k := strings.TrimLeft(
				strings.TrimPrefix(filename, dirname), "/")
			components := strings.Split(k, "/")
			if len(components) > 0 &&
				!utils.InString(files, components[0]) {
				name := utils.UnsanitizeComponent(components[0])

				if strings.HasSuffix(name, ".db") {
					continue
				}

				name_type, name := api.GetFileStorePathTypeFromExtension(name)
				child := path.AddChild(name).SetType(name_type)

				result = append(result, &vtesting.MockFileInfo{
					Name_:     name,
					PathSpec_: child,
					FullPath_: child.AsClientPath(),
					Size_:     int64(len(v)),
				})
				files = append(files, name)
			}
		}
	}
	return result, nil
}

func (self *MemoryFileStore) Walk(
	root api.FSPathSpec, walkFn api.WalkFunc) error {
	children, err := self.ListDirectory(root)
	if err != nil {
		return err
	}

	for _, child_info := range children {
		full_path := root.AddChild(utils.UnsanitizeComponent(
			child_info.Name()))
		err1 := walkFn(full_path, child_info)
		if err1 == filepath.SkipDir {
			continue
		}

		err1 = self.Walk(full_path, walkFn)
		if err1 != nil {
			return err1
		}
	}

	return nil
}

func (self *MemoryFileStore) Delete(path api.FSPathSpec) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	filename := pathSpecToPath(path, self.config_obj)
	self.Trace("Delete", filename)
	self.Data.Delete(filename)
	return nil
}

func (self *MemoryFileStore) Trace(name, filename string) {
	return
	fmt.Printf("Trace MemoryFileStore: %v: %v\n", name, filename)
}

func (self *MemoryFileStore) Get(filename string) ([]byte, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, pres := self.Data.Get(cleanPathForWindows(filename))
	if pres {
		return res.([]byte), pres
	}
	return nil, false
}

func (self *MemoryFileStore) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Data = ordereddict.NewDict()
}

func pathSpecToPath(
	p api.FSPathSpec, config_obj *config_proto.Config) string {
	return cleanPathForWindows(p.AsFilestoreFilename(config_obj))
}

func cleanPathForWindows(result string) string {
	// Sanitize it on windows to convert back to a common format
	// for comparisons.
	if runtime.GOOS == "windows" {
		return path.Clean(strings.Replace(strings.TrimPrefix(
			result, path_specs.WINDOWS_LFN_PREFIX), "\\", "/", -1))
	}

	return result
}

func pathDirSpecToPath(p api.FSPathSpec,
	config_obj *config_proto.Config) string {
	return cleanPathForWindows(p.AsFilestoreDirectory(config_obj))
}
