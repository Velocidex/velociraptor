package utils

import (
	"bytes"
	"compress/zlib"
	"io"

	"github.com/pkg/errors"
)

func Compress(plain_text []byte) ([]byte, error) {
	var b bytes.Buffer
	w, err := zlib.NewWriterLevel(&b, zlib.BestSpeed)
	if err != nil {
		return nil, err
	}

	_, err = w.Write(plain_text)
	if err != nil {
		return nil, err
	}

	w.Close()

	return b.Bytes(), nil
}

func Uncompress(compressed []byte) ([]byte, error) {
	var reader io.Reader = bytes.NewReader(compressed)
	z, err := zlib.NewReader(reader)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer z.Close()

	return io.ReadAll(z)
}
