package utils

import (
	"bufio"
	"io"
)

type BufferCloser struct {
	*bufio.Writer

	fd io.WriteCloser
}

func (self *BufferCloser) Close() error {
	self.Flush()

	return self.fd.Close()
}

func NewBufferCloser(fd io.WriteCloser) *BufferCloser {
	return &BufferCloser{
		Writer: bufio.NewWriterSize(fd, 1024*1024),
		fd:     fd,
	}
}
