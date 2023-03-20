package utils

import (
	"io"

	errors "github.com/go-errors/errors"
)

type MemoryBuffer struct {
	MaxSize int
	offset  int
	buff    []byte
}

func (self *MemoryBuffer) Bytes() []byte {
	return self.buff
}

func (self *MemoryBuffer) Seek(offset int64, whence int) (int64, error) {
	newLoc := self.offset

	switch whence {
	case io.SeekStart:
		newLoc = int(offset)
	case io.SeekCurrent:
		newLoc += int(offset)
	case io.SeekEnd:
		newLoc = len(self.buff) + int(offset)
	}

	self.offset = newLoc

	return int64(self.offset), nil
}

func (self *MemoryBuffer) Write(buff []byte) (n int, err error) {
	// Do we have space? This is the end of the new buffer
	end_offset := self.offset + len(buff)
	if end_offset > self.MaxSize {
		return 0, errors.New("Memory buffer capacity exceeded")
	}

	if self.offset >= len(self.buff) {
		// Zero Pad the buffer to the offset
		for i := 0; i < self.offset-len(self.buff); i++ {
			self.buff = append(self.buff, 0)
		}
	}

	for _, c := range buff {
		if len(self.buff) <= self.offset {
			self.buff = append(self.buff, c)
		} else {
			self.buff[self.offset] = c
		}
		self.offset++
	}

	return len(buff), nil
}
