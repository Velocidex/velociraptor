package utils

import (
	"context"
	"io"
)

// An io.Copy() that respects context cancellations.
func Copy(ctx context.Context, dst io.Writer, src io.Reader) (n int, err error) {
	offset := 0
	buff := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return n, nil

		default:
			n, err = src.Read(buff)
			if err != nil && err != io.EOF {
				return offset, err
			}

			if n == 0 {
				return offset, nil
			}

			_, err = dst.Write(buff[:n])
			if err != nil {
				return offset, err
			}
			offset += n
		}
	}
}
