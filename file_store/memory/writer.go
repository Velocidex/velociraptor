package memory

import (
	"bytes"
	"encoding/binary"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

type MemoryWriter struct {
	buf               []byte
	memory_file_store *MemoryFileStore
	pathSpec_         api.FSPathSpec
	filename          string
	closed            bool
	completion        func()
}

func (self *MemoryWriter) Size() (int64, error) {
	reader, err := self.memory_file_store.ReadFile(
		self.pathSpec_.SetType(api.PATH_TYPE_FILESTORE_CHUNK_INDEX))
	if err != nil {
		return int64(len(self.buf)), nil
	}
	defer reader.Close()

	// Get the last chunk.
	chunk_idx := api.NewChunkIndex(reader)
	chunk, err := chunk_idx.Get(chunk_idx.Len() - 1)
	if err != nil {
		return 0, err
	}

	// Return the size in the uncompressed logical file.
	return int64(chunk.FileOffset + chunk.UncompressedLength), nil
}

func (self *MemoryWriter) Update(data []byte, offset int64) error {
	defer api.InstrumentWithDelay("update", "MemoryWriter", nil)()

	err := self._Flush()
	if err != nil {
		return err
	}

	buff, ok := self.memory_file_store.Get(self.filename)
	if !ok {
		return utils.NotFoundError
	}

	if offset >= int64(len(buff)) {
		return utils.NotFoundError
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

// Assumption: We do not support mixing compressed and uncompressed
// files.
func (self *MemoryWriter) WriteCompressed(
	data []byte,
	logical_offset uint64,
	uncompressed_size int) (int, error) {

	buf := &bytes.Buffer{}

	err := binary.Write(buf, binary.LittleEndian, &api.CompressedChunk{
		FileOffset:         int64(logical_offset),
		ChunkOffset:        int64(len(self.buf)),
		CompressedLength:   int64(len(data)),
		UncompressedLength: int64(uncompressed_size),
	})
	if err != nil {
		return 0, err
	}

	writer, err := self.memory_file_store.WriteFile(
		self.pathSpec_.SetType(api.PATH_TYPE_FILESTORE_CHUNK_INDEX))
	if err != nil {
		return 0, err
	}
	defer writer.Close()

	_, err = writer.Write(buf.Bytes())
	if err != nil {
		return 0, err
	}

	return self.Write(data)
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
	self.buf = nil
	self.memory_file_store.mu.Unlock()

	return self.memory_file_store.Delete(
		self.pathSpec_.SetType(api.PATH_TYPE_FILESTORE_CHUNK_INDEX))
}
