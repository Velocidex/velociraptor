package reporting

import (
	"encoding/binary"
	"io"
	"time"

	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

type ContainerResultSetWriter struct {
	idx_fd io.WriteCloser
	fd     io.WriteCloser
	offset uint64
}

func (self *ContainerResultSetWriter) Close() {
	self.idx_fd.Close()
	self.fd.Close()
}

func (self *ContainerResultSetWriter) WriteJSONL(b []byte) (int, error) {
	value := self.offset | (1 << 40)
	err := binary.Write(self.idx_fd, binary.LittleEndian, value)
	if err != nil {
		return 0, err
	}

	n, err := self.fd.Write(b)
	if err != nil {
		return n, err
	}

	self.offset += uint64(n)

	return n, nil
}

func (self *ContainerResultSetWriter) Write(row vfilter.Row) error {
	value := self.offset | (1 << 40)
	err := binary.Write(self.idx_fd, binary.LittleEndian, value)
	if err != nil {
		return err
	}

	serialized, err := json.Marshal(row)
	if err != nil {
		return err
	}

	n, err := self.fd.Write(serialized)
	if err != nil {
		return err
	}

	self.offset += uint64(n)

	n, err = self.fd.Write([]byte("\n"))
	if err != nil {
		return err
	}

	self.offset += uint64(n)

	return nil
}

func NewResultSetWriter(container *Container, filename string) (
	*ContainerResultSetWriter, error) {

	fd, err := container.Create(filename, time.Time{})
	if err != nil {
		return nil, err
	}

	idx_fd, err := container.Create(filename+".index", time.Time{})
	if err != nil {
		fd.Close()
		return nil, err
	}

	return &ContainerResultSetWriter{
		fd:     fd,
		idx_fd: idx_fd,
	}, nil
}
