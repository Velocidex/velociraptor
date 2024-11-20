package memory

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

var (
	// Only used for tests.
	Test_memory_file_store *MemoryFileStore
)

func ResetMemoryFileStore() {
	mu.Lock()
	defer mu.Unlock()

	Test_memory_file_store = nil
}

func NewMemoryFileStore(config_obj *config_proto.Config) *MemoryFileStore {
	mu.Lock()
	defer mu.Unlock()

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil
	}

	if Test_memory_file_store == nil {
		Test_memory_file_store = &MemoryFileStore{
			Data:       ordereddict.NewDict(),
			Paths:      ordereddict.NewDict(),
			db:         db,
			config_obj: config_obj,
		}
	}

	// Sanitize the FilestoreDirectory
	if config_obj.Datastore.FilestoreDirectory != "" &&
		strings.HasSuffix(config_obj.Datastore.FilestoreDirectory, "/") {
		config_obj.Datastore.FilestoreDirectory = strings.TrimSuffix(
			config_obj.Datastore.FilestoreDirectory, "/")
	}

	return Test_memory_file_store
}

type MemoryReader struct {
	pathSpec_ api.FSPathSpec
	filename  string
	offset    int
	closed    bool

	memory_file_store *MemoryFileStore
}

func (self *MemoryReader) Read(buf []byte) (int, error) {
	defer api.InstrumentWithDelay("read", "MemoryReader", nil)()

	fs_buf, pres := self.memory_file_store.Get(self.filename)
	if !pres {
		return 0, os.ErrNotExist
	}

	if self.offset >= len(fs_buf) {
		return 0, io.EOF
	}

	to_read := len(buf)
	if self.offset+to_read > len(fs_buf) {
		to_read = len(fs_buf) - self.offset
	}

	for i := 0; i < to_read; i++ {
		buf[i] = fs_buf[self.offset+i]
	}
	self.offset += to_read
	return to_read, nil
}

func (self *MemoryReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case os.SEEK_SET:
		self.offset = int(offset)
	case os.SEEK_CUR:
		offset += int64(self.offset)
	case os.SEEK_END:
		buff, ok := self.memory_file_store.Get(self.filename)
		if !ok {
			return 0, io.EOF
		}
		offset += int64(len(buff))
	}
	return offset, nil
}

func (self *MemoryReader) Close() error {
	if self.closed {
		panic("MemoryReader already closed")
	}
	self.closed = true
	return nil
}

func (self *MemoryReader) Stat() (api.FileInfo, error) {
	defer api.InstrumentWithDelay("stat", "MemoryReader", nil)()

	fs_buf, pres := self.memory_file_store.Get(self.filename)
	if !pres {
		return nil, os.ErrNotExist
	}

	return vtesting.MockFileInfo{
		Name_:     self.pathSpec_.Base(),
		PathSpec_: self.pathSpec_,
		FullPath_: self.filename,
		Size_:     int64(len(fs_buf)),
	}, nil
}

type MemoryWriter struct {
	buf               []byte
	memory_file_store *MemoryFileStore
	filename          string
	closed            bool
	completion        func()
}

func (self *MemoryWriter) Size() (int64, error) {
	self.memory_file_store.mu.Lock()
	defer self.memory_file_store.mu.Unlock()

	return int64(len(self.buf)), nil
}

func (self *MemoryWriter) Update(data []byte, offset int64) error {
	defer api.InstrumentWithDelay("update", "MemoryWriter", nil)()

	err := self._Flush()
	if err != nil {
		return err
	}

	buff, ok := self.memory_file_store.Get(self.filename)
	if !ok {
		return os.ErrNotExist
	}

	if offset >= int64(len(buff)) {
		return os.ErrNotExist
	}

	// Write the bytes into buffer offset
	for i := 0; i < len(data); i++ {
		if offset >= int64(len(buff)) {
			buff = append(buff, data[i])
		} else {
			buff[offset] = data[i]
		}
		offset++
	}

	self.memory_file_store.mu.Lock()
	defer self.memory_file_store.mu.Unlock()

	self.memory_file_store.Data.Set(self.filename, buff)
	self.buf = buff
	return nil
}

func (self *MemoryWriter) Write(data []byte) (int, error) {
	defer api.InstrumentWithDelay("write", "MemoryWriter", nil)()

	self.memory_file_store.mu.Lock()
	defer self.memory_file_store.mu.Unlock()

	self.buf = append(self.buf, data...)
	return len(data), nil
}

func (self *MemoryWriter) Flush() error {
	self.memory_file_store.mu.Lock()
	defer self.memory_file_store.mu.Unlock()

	return self._Flush()
}

func (self *MemoryWriter) _Flush() error {
	self.memory_file_store.Data.Set(self.filename, self.buf)
	self.buf = nil

	return nil
}

func (self *MemoryWriter) Close() error {
	if self.closed {
		// panic("MemoryWriter already closed")
	}
	self.closed = true

	// MemoryWriter is actually synchronous... Complete on close.
	if self.completion != nil &&
		!utils.CompareFuncs(self.completion, utils.SyncCompleter) {
		defer self.completion()
	}

	self.memory_file_store.mu.Lock()
	defer self.memory_file_store.mu.Unlock()

	self.memory_file_store.Data.Set(self.filename, self.buf)
	return nil
}

func (self *MemoryWriter) Truncate() error {
	defer api.InstrumentWithDelay("truncate", "MemoryWriter", nil)()

	self.memory_file_store.mu.Lock()
	defer self.memory_file_store.mu.Unlock()

	self.buf = nil
	return nil
}

type MemoryFileStore struct {
	mu sync.Mutex

	config_obj *config_proto.Config
	Data       *ordereddict.Dict
	Paths      *ordereddict.Dict
	db         datastore.DataStore
}

func (self *MemoryFileStore) Debug() {
	fmt.Println(self.DebugString())
}

func (self *MemoryFileStore) DebugString() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := "MemoryFileStore: \n"
	for _, k := range self.Data.Keys() {
		v_any, _ := self.Data.Get(k)
		v := v_any.([]byte)
		// Render index files especially
		if strings.HasSuffix(k, ".index") ||
			strings.HasSuffix(k, ".idx") ||
			strings.HasSuffix(k, ".tidx") {
			result += fmt.Sprintf("%v: %v\n", k, hex.Dump(v))
			continue
		}

		result += fmt.Sprintf("%v: %v\n", k, string(v))
	}

	return result
}

func (self *MemoryFileStore) ReadFile(path api.FSPathSpec) (api.FileReader, error) {
	defer api.InstrumentWithDelay("read_open", "MemoryFileStore", nil)()

	self.mu.Lock()
	defer self.mu.Unlock()

	filename := pathSpecToPath(self.db, self.config_obj, path)
	self.Trace("ReadFile", filename)
	_, pres := self.Data.Get(filename)
	if pres {
		return &MemoryReader{
			pathSpec_:         path,
			filename:          filename,
			memory_file_store: self,
		}, nil
	}

	return nil, os.ErrNotExist
}

func (self *MemoryFileStore) WriteFile(path api.FSPathSpec) (api.FileWriter, error) {
	return self.WriteFileWithCompletion(path, utils.SyncCompleter)
}

func (self *MemoryFileStore) WriteFileWithCompletion(
	path api.FSPathSpec, completion func()) (api.FileWriter, error) {

	defer api.InstrumentWithDelay("write_open", "MemoryFileStore", nil)()

	self.mu.Lock()
	defer self.mu.Unlock()

	filename := pathSpecToPath(self.db, self.config_obj, path)
	self.Trace("WriteFile", filename)
	buf, pres := self.Data.Get(filename)
	if !pres {
		buf = []byte{}
	}
	self.Data.Set(filename, buf)
	self.Paths.Set(filename, path)

	return &MemoryWriter{
		buf:               buf.([]byte),
		memory_file_store: self,
		filename:          filename,
		completion:        completion,
	}, nil
}

func (self *MemoryFileStore) StatFile(path api.FSPathSpec) (api.FileInfo, error) {
	defer api.InstrumentWithDelay("stat", "MemoryFileStore", nil)()

	self.mu.Lock()
	defer self.mu.Unlock()

	filename := pathSpecToPath(self.db, self.config_obj, path)
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
	defer api.InstrumentWithDelay("move", "MemoryFileStore", nil)()

	self.mu.Lock()
	defer self.mu.Unlock()

	src_filename := pathSpecToPath(self.db, self.config_obj, src)
	dest_filename := pathSpecToPath(self.db, self.config_obj, dest)
	buff, pres := self.Data.Get(src_filename)
	if !pres {
		return os.ErrNotExist
	}

	self.Data.Set(dest_filename, buff)
	self.Paths.Set(dest_filename, dest)
	self.Data.Delete(src_filename)
	return nil
}

func (self *MemoryFileStore) ListDirectory(root_path api.FSPathSpec) ([]api.FileInfo, error) {
	defer api.InstrumentWithDelay("list", "MemoryFileStore", nil)()
	self.mu.Lock()
	defer self.mu.Unlock()

	dirname := pathDirSpecToPath(self.db, self.config_obj, root_path)
	self.Trace("ListDirectory", dirname)

	root_components := root_path.Components()

	// Mapping between the base name and the files
	seen_files := make(map[string]api.FileInfo)
	seen_dirs := make(map[string]api.FileInfo)

	for _, filename := range self.Paths.Keys() {
		path_spec_any, _ := self.Paths.Get(filename)
		path_spec := path_spec_any.(api.FSPathSpec)

		if !path_specs.IsSubPath(root_path, path_spec) {
			continue
		}

		components := path_spec.Components()

		// It is a directory if there are more components so we add a
		// directory node.
		// Example:
		// Directory:
		// root_components = ["a"]
		// components = ["a", "b", "c"]
		//
		// File
		// root_components = ["a"]
		// components = ["a", "b"]
		if len(root_components)+1 == len(components) {
			v_any, pres := self.Data.Get(filename)
			if !pres {
				continue
			}
			v := v_any.([]byte)

			// This is a datastore path - skip
			if path_spec.Type() == api.PATH_TYPE_DATASTORE_PROTO {
				continue
			}

			new_child := &vtesting.MockFileInfo{
				Name_:     path_spec.Base(),
				PathSpec_: path_spec,
				FullPath_: path_spec.AsClientPath(),
				Size_:     int64(len(v)),
			}

			seen_files[filename] = new_child

			// This path is deeper than 1 path in.
		} else if len(components) > len(root_components)+1 {

			// The next level after root_path
			name := components[len(root_components)]
			child := root_path.AddUnsafeChild(name).
				SetType(api.PATH_TYPE_DATASTORE_DIRECTORY)

			new_child := &vtesting.MockFileInfo{
				Name_:     child.Base(),
				PathSpec_: child,
				FullPath_: child.AsClientPath(),
				Size_:     0,
				Mode_:     os.ModeDir,
			}

			seen_dirs[filename] = new_child
		}
	}

	// Add any directories
	for k, v := range seen_dirs {
		_, pres := seen_files[k]
		if !pres {
			seen_files[k] = v
		}
	}

	result := []api.FileInfo{}
	for _, v := range seen_files {
		result = append(result, v)
	}

	sort.Slice(result, func(i, j int) bool {
		ps1 := result[i].PathSpec()
		ps2 := result[j].PathSpec()
		return ps1.AsClientPath() < ps2.AsClientPath()
	})

	return result, nil
}

func (self *MemoryFileStore) Delete(path api.FSPathSpec) error {
	defer api.InstrumentWithDelay("delete", "MemoryFileStore", nil)()

	self.mu.Lock()
	defer self.mu.Unlock()

	filename := pathSpecToPath(self.db, self.config_obj, path)
	self.Trace("Delete", filename)
	self.Data.Delete(filename)
	self.Paths.Delete(filename)
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
	self.Paths = ordereddict.NewDict()

	// Next filestore will be pristine.
	ResetMemoryFileStore()
}

func (self *MemoryFileStore) Close() error {
	self.Clear()
	return nil
}

func pathSpecToPath(
	db datastore.DataStore,
	config_obj *config_proto.Config, p api.FSPathSpec) string {
	return cleanPathForWindows(datastore.AsFilestoreFilename(db, config_obj, p))
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

func pathDirSpecToPath(
	db datastore.DataStore,
	config_obj *config_proto.Config, p api.FSPathSpec) string {
	return cleanPathForWindows(
		datastore.AsFilestoreDirectory(db, config_obj, p))
}
