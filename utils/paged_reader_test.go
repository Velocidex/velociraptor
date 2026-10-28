package utils

import (
	"bytes"
	"errors"
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
		if n == 0 || err != nil && err != io.EOF {
			break
		}
		reads++
		result += string(buf[:n])
	}

	assert.Equal(t, result, data)

	// Since buf is smaller than pagesize we expects to read more but
	// generate less reads on the backing file.
	assert.Equal(t, reads, 5)
	assert.Equal(t, reader.counter, 3)
}

// Make sure that the pages that go on the free list are full size pages.
func TestPagedReaderFreeList(t *testing.T) {
	// Length 501
	data := strings.Repeat("0123456789", 50) + "A"
	reader := &CountingBuffer{Reader: bytes.NewReader([]byte(data))}

	paged_reader, err := NewPagedReader(reader, 10, 10)
	assert.NoError(t, err)

	buf := make([]byte, 10)

	// Read a bit at the end
	n, err := paged_reader.ReadAt(buf, int64(len(data)-6))
	assert.True(t, errors.Is(err, io.EOF))
	assert.Equal(t, n, 6)

	// Force the page to be returned to the freelist
	paged_reader.Flush()

	for _, f := range paged_reader.freelist.freelist {
		// All freelist pages should be full size.
		assert.Equal(t, 10, len(f))
	}

	// Next read will use the freelist page
	n, err = paged_reader.ReadAt(buf, int64(len(data)-6))
	assert.True(t, errors.Is(err, io.EOF))
	assert.Equal(t, n, 6)
}
