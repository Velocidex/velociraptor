package utils

import (
	"bytes"
	"testing"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestReaderAt(t *testing.T) {
	flat_file := MakeReaderAtter(bytes.NewReader([]byte("Hello world")))
	// Gap between words of 10 sparse bytes
	index := &actions_proto.Index{
		// First range "hello "
		// Second range - sparse 5 bytes
		// Third range - "world"
		Ranges: []*actions_proto.Range{
			{
				FileOffset:     0,
				OriginalOffset: 0,
				FileLength:     5,
				Length:         5,
			},
			{
				FileOffset:     5,
				OriginalOffset: 5,
				FileLength:     0,
				Length:         5,
			},
			{
				FileOffset:     5,
				OriginalOffset: 10,
				FileLength:     5,
				Length:         5,
			},
		},
	}

	reader := RangedReader{ReaderAt: flat_file, Index: index}
	buffer := make([]byte, 40)
	n, err := reader.ReadAt(buffer, 0)
	assert.NoError(t, err)
	assert.Equal(t, n, 15)
	assert.Equal(t, string(buffer[:n]), "Hello\x00\x00\x00\x00\x00 worl")

	// partial read across the first range
	n, err = reader.ReadAt(buffer, 3)
	assert.NoError(t, err)
	assert.Equal(t, n, 12)
	assert.Equal(t, string(buffer[:n]), "lo\x00\x00\x00\x00\x00 worl")

	// partial read across the last range
	n, err = reader.ReadAt(buffer, 12)
	assert.NoError(t, err)
	assert.Equal(t, n, 3)
	assert.Equal(t, string(buffer[:n]), "orl")

	// partial read inside one range
	n, err = reader.ReadAt(buffer[:3], 1)
	assert.NoError(t, err)
	assert.Equal(t, n, 3)
	assert.Equal(t, string(buffer[:n]), "ell")
}
