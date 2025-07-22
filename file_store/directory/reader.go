package directory

import (
	"context"
	"fmt"
	"io"
	"os"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
)

type CompressedDirectoryReader struct {
	chunkIndex *api.ChunkIndex
	reader     api.FileReader
	offset     int64

	chunkCache *cache.LRUCache
}

func (self *CompressedDirectoryReader) uncompressChunk(
	chunk *api.CompressedChunk) ([]byte, error) {

	key := fmt.Sprintf("%v", chunk.ChunkOffset)
	value, ok := self.chunkCache.Get(key)
	if ok {
		return value.([]byte), nil
	}

	compressed := make([]byte, chunk.CompressedLength)
	_, err := self.reader.Seek(chunk.ChunkOffset, os.SEEK_SET)
	if err != nil {
		return nil, err
	}

	n, err := self.reader.Read(compressed)
	if err != nil || int64(n) != chunk.CompressedLength {
		return nil, io.EOF
	}

	uncompressed, err := utils.Uncompress(context.Background(), compressed)
	if err != nil {
		return nil, io.EOF
	}

	self.chunkCache.Set(key, uncompressed)
	return uncompressed, nil
}

func (self *CompressedDirectoryReader) readPartial(buf []byte) (int, error) {
	chunk, err := self.chunkIndex.GetChunkForFileOffset(self.offset)
	if err != nil {
		return 0, io.EOF
	}

	uncompressed, err := self.uncompressChunk(chunk)
	if err != nil {
		return 0, err
	}

	offset_within_chunk := int(self.offset - chunk.FileOffset)
	to_read := len(uncompressed) - int(offset_within_chunk)
	if to_read > len(buf) {
		to_read = len(buf)
	}

	for i := 0; i < to_read; i++ {
		buf[i] = uncompressed[i+offset_within_chunk]
	}

	return to_read, nil
}

func (self *CompressedDirectoryReader) Read(buf []byte) (n int, err error) {
	defer api.InstrumentWithDelay("read", "DirectoryReader", nil)()

	if self.offset > self.chunkIndex.FileSize() {
		return 0, io.EOF
	}

	offset := 0
	for {
		if offset >= len(buf) {
			break
		}

		n, err = self.readPartial(buf[offset:])
		if err != nil || n == 0 {
			break
		}

		offset += n
		self.offset += int64(n)
	}

	if offset == 0 {
		return 0, io.EOF
	}

	return offset, err
}

func (self *CompressedDirectoryReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case os.SEEK_SET:
		self.offset = offset
		return offset, nil

	case os.SEEK_CUR:
		return int64(self.offset), nil

	case os.SEEK_END:
		self.offset = self.chunkIndex.FileSize() + offset
	}
	return self.offset, nil
}

func (self *CompressedDirectoryReader) Stat() (api.FileInfo, error) {
	defer api.InstrumentWithDelay("stat", "DirectoryReader", nil)()

	res, err := self.reader.Stat()
	if err != nil {
		return nil, err
	}

	return &SizeWrapper{
		FileInfo: res,
		size:     self.chunkIndex.FileSize(),
	}, nil
}

func (self *CompressedDirectoryReader) Close() error {
	return nil
}

type SizeWrapper struct {
	api.FileInfo
	size int64
}

func (self *SizeWrapper) Size() int64 {
	return self.size
}
