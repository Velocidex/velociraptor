package utils

import (
	"errors"
	"io"
)

type ReadSeekReaderAdapter struct {
	reader io.ReaderAt
	offset int64
}

func (self ReadSeekReaderAdapter) Close() error {
	return nil
}

func (self *ReadSeekReaderAdapter) Read(buf []byte) (int, error) {
	n, err := self.reader.ReadAt(buf, self.offset)
	self.offset += int64(n)
	return n, err
}

func (self *ReadSeekReaderAdapter) Seek(offset int64, whence int) (int64, error) {
	if whence != 0 {
		return 0, errors.New("Unsupported whence")
	}

	self.offset = offset
	return offset, nil
}

func NewReadSeekReaderAdapter(reader io.ReaderAt) *ReadSeekReaderAdapter {
	return &ReadSeekReaderAdapter{reader: reader}
}
