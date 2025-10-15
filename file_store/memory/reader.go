package memory

import (
	"io"
	"os"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

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
		return 0, utils.NotFoundError
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
		return nil, utils.NotFoundError
	}

	return vtesting.MockFileInfo{
		Name_:     self.pathSpec_.Base(),
		PathSpec_: self.pathSpec_,
		FullPath_: self.filename,
		Size_:     int64(len(fs_buf)),
	}, nil
}
