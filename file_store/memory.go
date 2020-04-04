package file_store

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
)

type MemoryReader struct {
	*bytes.Reader
}

func (self MemoryReader) Close() error {
	return nil
}

func (self MemoryReader) Stat() (os.FileInfo, error) {
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
func (self *MemoryWriter) Append(data []byte) error {
	self.buf = append(self.buf, data...)
	return nil
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

func (self *MemoryFileStore) ReadFile(filename string) (FileReader, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	data, pres := self.Data[filename]
	if pres {
		return MemoryReader{bytes.NewReader(data)}, nil
	}

	return nil, errors.New("Not found")
}

func (self *MemoryFileStore) WriteFile(filename string) (FileWriter, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	buf, pres := self.Data[filename]
	if !pres {
		buf = []byte{}
	}
	self.Data[filename] = buf

	return &MemoryWriter{
		memory_file_store: self,
		filename:          filename,
	}, nil
}

func (self *MemoryFileStore) StatFile(filename string) (*FileStoreFileInfo, error) {
	return &FileStoreFileInfo{}, nil
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
