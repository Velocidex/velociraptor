package utils

import (
	"context"
	"io"
)

// An io.Copy() that respects context cancellations.
func Copy(ctx context.Context, dst io.Writer, src io.Reader) (n int, err error) {
	buff := make([]byte, 1024*1024)
	for {
		select {
		case <-ctx.Done():
			return n, nil

		default:
			n, err = src.Read(buff)
			if err != nil && err != io.EOF {
				return n, err
			}

			if n == 0 {
				return n, nil
			}

			_, err = dst.Write(buff[:n])
			if err != nil {
				return n, err
			}
		}
	}
}
