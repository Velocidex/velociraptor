package utils

import "io"

type ReaderAtter struct {
	Reader io.ReadSeeker
}

func (self ReaderAtter) ReadAt(buf []byte, offset int64) (int, error) {
	self.Reader.Seek(offset, 0)
	return self.Reader.Read(buf)
}
