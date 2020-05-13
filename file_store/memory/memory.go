package memory

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/glob"
)

var (
	// Only used for tests.
	Test_memory_file_store *MemoryFileStore = &MemoryFileStore{
		Data: make(map[string][]byte)}
)

type MemoryReader struct {
	*bytes.Reader
}

func (self MemoryReader) Close() error {
	return nil
}

func (self MemoryReader) Stat() (glob.FileInfo, error) {
	return nil, errors.New("Not Implemented")
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
		return MemoryReader{bytes.NewReader(data)}, nil
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
	return &api.FileStoreFileInfo{}, nil
}

func (self *MemoryFileStore) ListDirectory(dirname string) ([]os.FileInfo, error) {
	return nil, nil
}

func (self *MemoryFileStore) Walk(root string, cb filepath.WalkFunc) error {
	return errors.New("Not implemented")
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
