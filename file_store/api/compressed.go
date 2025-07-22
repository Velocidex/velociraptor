package api

import (
	"bytes"
	"encoding/binary"
	"os"
	"sort"
	"unsafe"

	"www.velocidex.com/golang/velociraptor/utils"
)

// Compressed files contain a sequence of chunks, each compressed
// already. To keep track of these chunks we have a chunks file:

// Main file:
// <Magic>
// <chunk 1>
// <Magic>
// <chunk 2>

// Chunk file;
// CompressedChunk struct
// CompressedChunk struct

// By definition, the chunks are sequential (because file store files
// can only be appended to). Therefore it is possible to find the
// correct chunk that covers a particular file offset by binary search
// of the chunk file.

type CompressedChunk struct {
	// Offset of chunk in the logical uncompressed file
	FileOffset int64

	// Offset of chunk in the compressed stream.
	ChunkOffset int64

	// How large the chunk is in the compressed stream.
	CompressedLength int64

	// Total uncompressed size of this chunk.
	UncompressedLength int64

	// IV is used for encryption. If it is zero then no encryption is
	// applied.
	IV uint64
}

const (
	SizeofCompressedChunk = int(unsafe.Sizeof(CompressedChunk{}))
)

type ChunkIndex struct {
	reader FileReader
	cache  map[int]*CompressedChunk
}

func NewChunkIndex(reader FileReader) *ChunkIndex {
	return &ChunkIndex{
		reader: reader,
		cache:  make(map[int]*CompressedChunk),
	}
}

func (self *ChunkIndex) Len() int {
	st, err := self.reader.Stat()
	if err != nil {
		return 0
	}
	return int(st.Size()) / SizeofCompressedChunk
}

func (self *ChunkIndex) FileSize() int64 {
	chunk, err := self.Get(self.Len() - 1)
	if err != nil {
		return 0
	}
	return int64(chunk.FileOffset + chunk.UncompressedLength)
}

func (self *ChunkIndex) GetChunkForFileOffset(offset int64) (
	*CompressedChunk, error) {
	// Binary search compatible with older go releases.
	// Return the smallest chunk index which is below the desired file
	// offset.
	hit := sort.Search(self.Len(), func(index int) bool {
		chunk, err := self.Get(index)
		if err != nil {
			return false
		}

		return chunk.FileOffset > offset
	})

	// The next chunk should cover the required index
	return self.Get(hit - 1)
}

func (self *ChunkIndex) Get(index int) (*CompressedChunk, error) {
	if index < 0 || index > self.Len() {
		return nil, utils.NotFoundError
	}

	chunk, pres := self.cache[index]
	if pres {
		return chunk, nil
	}

	chunk = &CompressedChunk{}
	buff := make([]byte, SizeofCompressedChunk)

	_, err := self.reader.Seek(int64(index*SizeofCompressedChunk), os.SEEK_SET)
	if err != nil {
		return nil, err
	}

	_, err = self.reader.Read(buff)
	if err != nil {
		return nil, err
	}

	err = binary.Read(bytes.NewReader(buff), binary.LittleEndian, chunk)
	if err != nil {
		return nil, err
	}

	self.cache[index] = chunk

	return chunk, nil
}
