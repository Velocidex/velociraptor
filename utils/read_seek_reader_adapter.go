package utils

import (
	"errors"
	"io"
)

type Closer interface {
	Close() error
}

type ReadSeekReaderAdapter struct {
	reader io.ReaderAt
	offset int64
	size   int64
	eof    bool
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
	if self.eof {
		return 0, io.EOF
	}

	if self.offset < 0 {
		return 0, IOError
	}

	// This read would exceed the size, so we read up to the size and
	// flag the eof.
	if self.size > 0 && self.offset+int64(len(buf)) > self.size {
		buf = buf[:self.size-self.offset]
		self.eof = true
	}

	n, err := self.reader.ReadAt(buf, self.offset)
	if errors.Is(err, io.EOF) {
		self.eof = true
	}
	self.offset += int64(n)

	return n, err
}

func (self *ReadSeekReaderAdapter) SetSize(size int64) {
	self.size = size
}

func (self *ReadSeekReaderAdapter) IsSeekable() bool {
	return true
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
