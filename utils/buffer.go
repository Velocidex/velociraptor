package utils

import (
	"bufio"
	"fmt"
	"io"
)

type BufferCloser struct {
	*bufio.Writer

	fd io.WriteCloser
}

func (self *BufferCloser) Close() error {
	err := self.Writer.Flush()
	if err != nil {
		return err
	}

	return self.fd.Close()
}

func (self *BufferCloser) GoString() string {
	return fmt.Sprintf("BufferCloser: %v on %#v", self.Buffered(), self.fd)
}

func NewBufferCloser(fd io.WriteCloser) *BufferCloser {
	return &BufferCloser{
		Writer: bufio.NewWriterSize(fd, 1024*1024),
		fd:     fd,
	}
}
