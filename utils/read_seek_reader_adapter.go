package utils

import (
	"errors"
	"io"
	"sync"
)

type Closer interface {
	Close() error
}

type ReadSeekReaderAdapter struct {
	mu     sync.Mutex
	reader io.ReaderAt
	offset int64
	size   int64
	eof    bool

	closer func()
}

func (self *ReadSeekReaderAdapter) Close() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Try to close our delegate if possible
	switch t := self.reader.(type) {
	case Flusher:
		t.Flush()

	case Closer:
		t.Close()

	default:
	}

	if self.closer != nil {
		self.closer()
	}

	return nil
}

func (self *ReadSeekReaderAdapter) Read(buf []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.eof {
		return 0, io.EOF
	}

	if self.offset < 0 {
		return 0, IOError
	}

	// This read would exceed the size, so we read up to the size and
	// flag the eof.
	if self.size > 0 && self.offset+int64(len(buf)) > self.size {
		if self.size-self.offset < 0 {
			return 0, io.EOF
		}
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
	self.mu.Lock()
	defer self.mu.Unlock()

	self.size = size
}

func (self *ReadSeekReaderAdapter) IsSeekable() bool {
	return true
}

func (self *ReadSeekReaderAdapter) Seek(offset int64, whence int) (int64, error) {
	if whence != 0 {
		return 0, errors.New("Unsupported whence")
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	self.offset = offset
	return offset, nil
}

func NewReadSeekReaderAdapter(reader io.ReaderAt, closer func()) *ReadSeekReaderAdapter {
	return &ReadSeekReaderAdapter{
		reader: reader,
		closer: closer,
	}
}
