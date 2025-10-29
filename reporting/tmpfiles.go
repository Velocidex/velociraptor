package reporting

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"os"

	"github.com/Velocidex/zip"
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

// A tempfile that writes to memory until a certain zip, then switches
// to file based if the size is exceeded. This saves unnecessary IO
// for small files.
type BufferedTmpFile struct {
	file     zip.TempFile
	buffer   *bytes.Buffer
	max_size int
}

func (self *BufferedTmpFile) Write(buff []byte) (int, error) {
	if self.file != nil {
		return self.file.Write(buff)
	}

	// Switch to tmpfile backing if the write will exceed the buffer
	// size.
	if self.buffer.Len()+len(buff) > self.max_size {
		new_tmpfile, err := zip.DefaultTmpfileProvider(0).TempFile()
		if err != nil {
			return 0, err
		}

		self.file = new_tmpfile
		_, err = new_tmpfile.Write(self.buffer.Bytes())
		if err != nil {
			return 0, err
		}
		n, err := new_tmpfile.Write(buff)
		if err != nil {
			return 0, err
		}
		self.buffer = nil

		return n, err
	}

	return self.buffer.Write(buff)
}

func (self *BufferedTmpFile) Close() error {
	self.buffer = nil
	if self.file != nil {
		return self.file.Close()
	}
	return nil
}

// Returns a reader to the tmp file.
func (self *BufferedTmpFile) Open() (io.ReadCloser, error) {
	if self.file != nil {
		return self.file.Open()
	}
	return ioutil.NopCloser(self.buffer), nil
}

func (self *BufferedTmpFile) Remove() {
	if self.file != nil {
		self.file.Remove()
	}
}

func (self *BufferedTmpFile) Name() string {
	if self.file != nil {
		return self.file.Name()
	}
	return "memory"
}

type TmpfileProvider int

func (self TmpfileProvider) TempFile() (zip.TempFile, error) {
	res := &BufferedTmpFile{
		max_size: 1024 * 1204,
		buffer:   &bytes.Buffer{},
	}
	res.buffer.Grow(res.max_size)
	return res, nil
}

func init() {
	zip.SetTmpfileProvider(TmpfileProvider(0))
}
