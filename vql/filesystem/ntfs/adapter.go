package ntfs

import (
	"errors"
	"io"
	"os"

	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
)

type readSeekReaderAdapter struct {
	reader io.ReaderAt
	offset int64
	info   glob.FileInfo
}

func (self readSeekReaderAdapter) Close() error {
	return nil
}

func (self *readSeekReaderAdapter) Read(buf []byte) (int, error) {
	n, err := self.reader.ReadAt(buf, self.offset)
	self.offset += int64(n)
	return n, err
}

func (self *readSeekReaderAdapter) Seek(offset int64, whence int) (int64, error) {
	if whence != 0 {
		return 0, errors.New("Unsupported whence")
	}

	self.offset = offset
	return offset, nil
}

func (self *readSeekReaderAdapter) Stat() (os.FileInfo, error) {
	if utils.IsNil(self.info) {
		return nil, errors.New("Not supported")
	}
	return self.info, nil
}
