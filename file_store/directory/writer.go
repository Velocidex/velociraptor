package directory

import (
	"bytes"
	"encoding/binary"
	"os"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	// On windows all file paths must be prefixed by this to
	// support long paths.
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

type DirectoryFileWriter struct {
	Fd      *os.File
	ChunkFd *os.File
	path    api.FSPathSpec

	config_obj *config_proto.Config
	db         datastore.DataStore
	completion func()
}

func (self *DirectoryFileWriter) Size() (int64, error) {
	if self.ChunkFd != nil {
		// Get the last chunk.
		chunk_idx := api.NewChunkIndex(&api.FileAdapter{
			File: self.ChunkFd,
		})
		chunk, err := chunk_idx.Get(chunk_idx.Len() - 1)
		if err != nil {
			return 0, err
		}

		// Return the size in the uncompressed logical file.
		return int64(chunk.FileOffset + chunk.UncompressedLength), nil
	}

	return self.Fd.Seek(0, os.SEEK_END)
}

func (self *DirectoryFileWriter) Update(data []byte, offset int64) error {
	_, err := self.Fd.Seek(offset, os.SEEK_SET)
	if err != nil {
		return err
	}

	_, err = self.Fd.Write(data)
	return err
}

func (self *DirectoryFileWriter) WriteCompressed(
	data []byte,
	logical_offset uint64,
	uncompressed_size int) (n int, err error) {

	// Create the chunk index if needed
	if self.ChunkFd == nil {
		chunk_file_path := datastore.AsFilestoreFilename(
			self.db, self.config_obj, self.path.
				SetType(api.PATH_TYPE_FILESTORE_CHUNK_INDEX))

		err = checkPath(chunk_file_path)
		if err != nil {
			return 0, err
		}

		self.ChunkFd, err = os.OpenFile(chunk_file_path,
			os.O_RDWR|os.O_CREATE, 0600)
		if err != nil {
			return 0, err
		}
	}

	// Write the index on the end
	chunk_offset, err := self.Fd.Seek(0, os.SEEK_END)
	if err != nil {
		return 0, err
	}

	buf := &bytes.Buffer{}
	err = binary.Write(buf, binary.LittleEndian, &api.CompressedChunk{
		FileOffset:         int64(logical_offset),
		ChunkOffset:        int64(chunk_offset),
		CompressedLength:   int64(len(data)),
		UncompressedLength: int64(uncompressed_size),
	})
	if err != nil {
		return 0, err
	}

	// Write the chunk index.
	_, err = self.ChunkFd.Seek(0, os.SEEK_END)
	if err != nil {
		return 0, err
	}

	_, err = self.ChunkFd.Write(buf.Bytes())
	if err != nil {
		return 0, err
	}

	// Write the chunk
	return self.Write(data)
}

func (self *DirectoryFileWriter) Write(data []byte) (int, error) {
	defer api.InstrumentWithDelay("write", "DirectoryFileWriter", nil)()

	_, err := self.Fd.Seek(0, os.SEEK_END)
	if err != nil {
		return 0, err
	}

	return self.Fd.Write(data)
}

func (self *DirectoryFileWriter) Truncate() error {
	if self.ChunkFd != nil {
		filename := self.ChunkFd.Name()

		err := self.ChunkFd.Truncate(0)
		self.ChunkFd.Close()
		os.Remove(filename)

		if err != nil {
			return err
		}
		self.ChunkFd = nil
	}

	return self.Fd.Truncate(0)
}

func (self *DirectoryFileWriter) Flush() error { return nil }

func (self *DirectoryFileWriter) Close() error {
	if self.ChunkFd != nil {
		self.ChunkFd.Close()
	}

	err := self.Fd.Close()

	// DirectoryFileWriter is synchronous... complete on Close()
	if self.completion != nil &&
		!utils.CompareFuncs(self.completion, utils.SyncCompleter) {
		self.completion()
	}
	return err
}
