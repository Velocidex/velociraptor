package memory

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

var (
	// Only used for tests.
	Test_memory_file_store *MemoryFileStore = &MemoryFileStore{
		Data: ordereddict.NewDict(),
	}
)

type MemoryReader struct {
	*bytes.Reader
	filename string
}

func (self MemoryReader) Close() error {
	return nil
}

func (self MemoryReader) Stat() (glob.FileInfo, error) {
	return vtesting.MockFileInfo{
		Name_:     self.filename,
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

	Data *ordereddict.Dict
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

func (self *MemoryFileStore) ReadFile(filename string) (api.FileReader, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	data_any, pres := self.Data.Get(filename)
	if pres {
		data := data_any.([]byte)
		return MemoryReader{
			Reader:   bytes.NewReader(data),
			filename: filename,
		}, nil
	}

	return nil, errors.New("Not found")
}

func (self *MemoryFileStore) WriteFile(filename string) (api.FileWriter, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

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

func (self *MemoryFileStore) StatFile(filename string) (os.FileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	buff, pres := self.Data.Get(filename)
	if !pres {
		return nil, os.ErrNotExist
	}

	return &vtesting.MockFileInfo{
		Name_:     path.Base(filename),
		FullPath_: filename,
		Size_:     int64(len(buff.([]byte))),
	}, nil
}

func (self *MemoryFileStore) Move(src, dest string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	buff, pres := self.Data.Get(src)
	if !pres {
		return os.ErrNotExist
	}

	self.Data.Set(dest, buff)
	self.Data.Delete(src)
	return nil
}

func (self *MemoryFileStore) ListDirectory(dirname string) ([]os.FileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []os.FileInfo{}
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
				result = append(result, &vtesting.MockFileInfo{
					Name_:     components[0],
					FullPath_: path.Join(dirname, components[0]),
					Size_:     int64(len(v)),
				})
				files = append(files, components[0])
			}
		}
	}
	return result, nil
}

func (self *MemoryFileStore) Walk(root string, walkFn filepath.WalkFunc) error {
	children, err := self.ListDirectory(root)
	if err != nil {
		return err
	}

	for _, child_info := range children {
		full_path := path.Join(root, child_info.Name())
		err1 := walkFn(full_path, child_info, err)
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

func (self *MemoryFileStore) Delete(filename string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Data.Delete(filename)
	return nil
}

func (self *MemoryFileStore) Get(filename string) ([]byte, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, pres := self.Data.Get(filename)
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
