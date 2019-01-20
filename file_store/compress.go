package file_store

import (
	"errors"
	"fmt"
	"io"
)

type SeekableGzip struct {
	io.Reader

	backing_file io.Closer
}

func (self *SeekableGzip) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		if offset == 0 {
			return 0, nil
		}

	}
	return 0, errors.New(fmt.Sprintf(
		"Seeking to %v (%v) not supported on compressed files.",
		offset, whence))
}

func (self *SeekableGzip) Close() error {
	return self.backing_file.Close()
}
