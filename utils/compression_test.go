package utils

import (
	"context"
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestCompression(t *testing.T) {
	test_str := []byte("hello world")
	buffer, err := Compress(test_str)
	assert.NoError(t, err)

	uncompressed, err := Uncompress(context.Background(), buffer)
	assert.NoError(t, err)

	assert.Equal(t, string(test_str), string(uncompressed))
}
