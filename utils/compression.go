package utils

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"

	errors "github.com/pkg/errors"
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

func Uncompress(
	ctx context.Context, compressed []byte) ([]byte, error) {

	result := bytes.NewBuffer(make([]byte, 0, len(compressed)*2))
	var reader io.Reader = bytes.NewReader(compressed)
	z, err := zlib.NewReader(reader)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer z.Close()

	_, err = Copy(ctx, result, z)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return result.Bytes(), nil
}
