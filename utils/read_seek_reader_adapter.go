package utils

import (
	"errors"
	"io"
)

type Flusher interface {
	Flush()
}

type Closer interface {
	Close() error
}

type ReadSeekReaderAdapter struct {
	reader io.ReaderAt
	offset int64
}

func (self ReadSeekReaderAdapter) Close() error {
	// Try to close our delegate if possible
	switch t := self.reader.(type) {
	case Flusher:
		t.Flush()

	case Closer:
		t.Close()

	default:
	}
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
