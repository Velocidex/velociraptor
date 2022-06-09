package raw_registry

import (
	"bytes"
	"io"

	"www.velocidex.com/golang/velociraptor/accessors"
)

type ValueBuffer struct {
	io.ReadSeeker
	info accessors.FileInfo
}

func (self *ValueBuffer) Stat() (accessors.FileInfo, error) {
	return self.info, nil
}

func (self *ValueBuffer) Close() error {
	return nil
}

func NewValueBuffer(buf []byte, stat accessors.FileInfo) *ValueBuffer {
	return &ValueBuffer{
		bytes.NewReader(buf),
		stat,
	}
}
