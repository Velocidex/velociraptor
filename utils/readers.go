package utils

import "io"

type ReaderAtter struct {
	Reader io.ReadSeeker
}

func (self ReaderAtter) ReadAt(buf []byte, offset int64) (int, error) {
	self.Reader.Seek(offset, 0)
	return self.Reader.Read(buf)
}

type BufferReaderAt struct {
	Buffer []byte
}

func (self *BufferReaderAt) ReadAt(buf []byte, offset int64) (int, error) {
	to_read := int64(len(buf))
	if offset < 0 {
		to_read += offset
		offset = 0
	}

	if offset+to_read > int64(len(self.Buffer)) {
		to_read = int64(len(self.Buffer)) - offset
	}

	if to_read < 0 {
		return 0, nil
	}

	n := copy(buf, self.Buffer[offset:offset+to_read])

	return n, nil
}
