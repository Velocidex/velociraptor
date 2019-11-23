package file_store

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
)

type CloserReader struct {
	*bytes.Reader
}

func (self CloserReader) Close() error {
	return nil
}

func (self CloserReader) Stat() (os.FileInfo, error) {
	return nil, errors.New("Not Implemented")
}

type CloserWriter struct {
	*WriterSeeker

	memory_file_store *MemoryFileStore
	filename          string
}

func (self CloserWriter) Close() error {
	self.memory_file_store.mu.Lock()
	defer self.memory_file_store.mu.Unlock()

	self.memory_file_store.Data[self.filename] = self.buf
	return nil
}

func (self CloserWriter) Truncate(size int64) error {
	self.buf = self.buf[:size]
	return nil
}

type MemoryFileStore struct {
	mu sync.Mutex

	Data map[string][]byte
}

func (self *MemoryFileStore) ReadFile(filename string) (ReadSeekCloser, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	data, pres := self.Data[filename]
	if pres {
		return CloserReader{bytes.NewReader(data)}, nil
	}

	return nil, errors.New("Not found")
}

func (self *MemoryFileStore) WriteFile(filename string) (WriteSeekCloser, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	buf := make([]byte, 0)
	self.Data[filename] = buf

	return CloserWriter{&WriterSeeker{buf, 0}, self, filename}, nil
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
