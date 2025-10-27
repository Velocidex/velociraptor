package reporting

import (
	"bufio"
	"os"
)

type BufferedCloser struct {
	*bufio.Writer
	fd *os.File
}

func (self *BufferedCloser) Name() string {
	return self.fd.Name()
}

func (self *BufferedCloser) Close() error {
	self.Writer.Flush()
	return self.fd.Close()
}

func NewBufferedCloser(fd *os.File) *BufferedCloser {
	return &BufferedCloser{
		Writer: bufio.NewWriterSize(fd, 1024*1204),
		fd:     fd,
	}
}
