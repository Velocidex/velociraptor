package utils

import (
	"context"
	"io"
	"sync"
)

var (
	pool = sync.Pool{
		New: func() interface{} {
			buffer := make([]byte, 32*1024)
			return &buffer
		},
	}
)

// An io.Copy() that respects context cancellations.
func Copy(ctx context.Context, dst io.Writer, src io.Reader) (n int, err error) {
	offset := 0
	buff := pool.Get().(*[]byte)
	defer pool.Put(buff)

	for {
		select {
		case <-ctx.Done():
			return n, nil

		default:
			n, err = src.Read(*buff)
			if err != nil && err != io.EOF {
				return offset, err
			}

			if n == 0 {
				return offset, nil
			}

			_, err = dst.Write((*buff)[:n])
			if err != nil {
				return offset, err
			}
			offset += n
		}
	}
}

func CopyN(ctx context.Context, dst io.Writer, src io.Reader, count int64) (
	n int, err error) {
	offset := 0
	buff := pool.Get().(*[]byte)
	defer pool.Put(buff)

	for count > 0 {
		select {
		case <-ctx.Done():
			return n, nil

		default:
			read_buff := *buff
			if count < int64(len(read_buff)) {
				read_buff = read_buff[:count]
			}

			n, err = src.Read(read_buff)
			if err != nil && err != io.EOF {
				return offset, err
			}

			if n == 0 {
				return offset, nil
			}

			_, err = dst.Write(read_buff[:n])
			if err != nil {
				return offset, err
			}
			offset += n
			count -= int64(n)
		}
	}
	return offset, nil
}

func MemCpy(out []byte, in []byte) int {
	length := len(in)
	if length > len(out) {
		length = len(out)

	}
	copy(out, in[:length])
	return length
}

func CopySlice(in []string) []string {
	result := make([]string, len(in))
	copy(result, in)
	return result
}
