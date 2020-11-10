package memory

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

var (
	// Only used for tests.
	Test_memory_file_store *MemoryFileStore = &MemoryFileStore{
		Data: make(map[string][]byte)}
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

	self.memory_file_store.Data[self.filename] = self.buf
	return nil
}

func (self *MemoryWriter) Truncate() error {
	self.buf = nil
	return nil
}

type MemoryFileStore struct {
	mu sync.Mutex

	Data map[string][]byte
}

func (self *MemoryFileStore) Debug() {
	self.mu.Lock()
	defer self.mu.Unlock()

	fmt.Printf("MemoryFileStore: \n")
	for k, v := range self.Data {
		fmt.Printf("%v: %v\n", k, string(v))
	}
}

func (self *MemoryFileStore) ReadFile(filename string) (api.FileReader, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	data, pres := self.Data[filename]
	if pres {
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

	buf, pres := self.Data[filename]
	if !pres {
		buf = []byte{}
	}
	self.Data[filename] = buf

	return &MemoryWriter{
		buf:               buf,
		memory_file_store: self,
		filename:          filename,
	}, nil
}

func (self *MemoryFileStore) StatFile(filename string) (os.FileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	buff, pres := self.Data[filename]
	if !pres {
		return nil, os.ErrNotExist
	}

	return &vtesting.MockFileInfo{
		Name_:     path.Base(filename),
		FullPath_: filename,
		Size_:     int64(len(buff)),
	}, nil
}

func (self *MemoryFileStore) ListDirectory(dirname string) ([]os.FileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	files := []string{}
	for filename := range self.Data {
		if strings.HasPrefix(filename, dirname) {
			k := strings.TrimLeft(strings.TrimPrefix(filename, dirname), "/")
			components := strings.Split(k, "/")
			if len(components) > 0 &&
				!utils.InString(files, components[0]) {
				files = append(files, components[0])
			}
		}
	}
	result := []os.FileInfo{}
	for _, file := range files {
		result = append(result, &vtesting.MockFileInfo{
			Name_:     file,
			FullPath_: path.Join(dirname, file),
		})
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

	delete(self.Data, filename)
	return nil
}

func (self *MemoryFileStore) Get(filename string) ([]byte, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, pres := self.Data[filename]
	return res, pres
}

func (self *MemoryFileStore) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Data = make(map[string][]byte)
}
