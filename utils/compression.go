package utils

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"io"

	errors "github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/constants"
)

func Compress(plain_text []byte) ([]byte, error) {
	var b bytes.Buffer
	w, err := zlib.NewWriterLevel(&b, zlib.BestSpeed)
	if err != nil {
		return nil, err
	}

	_, err = w.Write([]byte(plain_text))
	if err != nil {
		return nil, err
	}

	w.Close()

	return b.Bytes(), nil
}

func GzipUncompress(raw []byte) ([]byte, error) {
	rb := bytes.NewReader(raw)
	r, err := gzip.NewReader(rb)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(make([]byte, 0, bytes.MinRead))
	_, err = buf.ReadFrom(r)
	return buf.Bytes(), err
}

func Uncompress(
	ctx context.Context, compressed []byte) ([]byte, error) {

	return UncompressWithLimit(ctx, compressed, constants.MEMORY_LARGE)
}

func UncompressWithLimit(
	ctx context.Context, compressed []byte, max_size int64) ([]byte, error) {

	// Allocate a reasonable initial buffer. The decompression step
	// below may increase it as required.
	result := bytes.NewBuffer(make([]byte, 0, len(compressed)*2))
	var reader io.Reader = bytes.NewReader(compressed)
	z, err := zlib.NewReader(reader)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}
	defer z.Close()

	// Copy a bit more than we are supposed to so we can detect if we
	// hit the limit.
	n, err := CopyN(ctx, result, z, max_size+1)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	// There is more data than can fit in the max_size.
	if int64(n) > max_size {
		return nil, MemoryError
	}

	return result.Bytes(), nil
}
