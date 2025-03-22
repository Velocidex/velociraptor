package utils

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type CountingBuffer struct {
	*bytes.Reader
	counter int
}

func (self *CountingBuffer) ReadAt(buf []byte, offset int64) (int, error) {
	self.counter++
	return self.Reader.ReadAt(buf, offset)
}

func TestPagedReader(t *testing.T) {
	// Length 50
	data := strings.Repeat("0123456789", 5)
	reader := &CountingBuffer{Reader: bytes.NewReader([]byte(data))}

	// pagesize is prime to ensure a short read at the end
	paged_reader, err := NewPagedReader(reader, 21, 10)
	assert.NoError(t, err)

	result := ""
	buf := make([]byte, 11)
	fd := NewReadSeekReaderAdapter(paged_reader, nil)
	reads := 0
	for {
		n, err := fd.Read(buf)
		if n == 0 || err == io.EOF {
			break
		}
		assert.NoError(t, err)

		reads++
		result += string(buf[:n])
	}

	assert.Equal(t, result, data)

	// Since buf is smaller than pagesize we expects to read more but
	// generate less reads on the backing file.
	assert.Equal(t, reads, 5)
	assert.Equal(t, reader.counter, 3)
}
