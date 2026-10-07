package utils

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Reading past the end with Read() sets a sticky EOF flag. ReadAt()
// must not be affected by it.
func TestReadSeekReaderAdapterReadAt(t *testing.T) {
	data := []byte("0123456789")
	adapter := NewReadSeekReaderAdapter(bytes.NewReader(data), nil)
	adapter.SetSize(int64(len(data)))

	// Exhaust the reader with Read().
	_, err := io.ReadAll(adapter)
	assert.NoError(t, err)

	n, err := adapter.Read(make([]byte, 1))
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)

	// ReadAt still works anywhere in the file.
	buf := make([]byte, 4)
	n, err = adapter.ReadAt(buf, 2)
	assert.NoError(t, err)
	assert.Equal(t, "2345", string(buf[:n]))

	// A read crossing the size is cut short with EOF.
	n, err = adapter.ReadAt(buf, 8)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, "89", string(buf[:n]))

	// Reading past the end does not break later reads.
	n, err = adapter.ReadAt(buf, 20)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)

	n, err = adapter.ReadAt(buf, 0)
	assert.NoError(t, err)
	assert.Equal(t, "0123", string(buf[:n]))

	// MakeReaderAtter uses the stateless ReadAt.
	_, ok := MakeReaderAtter(adapter).(*ReadSeekReaderAdapter)
	assert.True(t, ok)
}
