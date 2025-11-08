package utils

import (
	"context"
	"io"
	"io/ioutil"
	"sync"

	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/vfilter"
)

var (
	pool = sync.Pool{
		New: func() interface{} {
			buffer := make([]byte, 1024*1024)
			return &buffer
		},
	}
)

func ReadAllWithLimit(
	fd io.Reader, limit int) ([]byte, error) {

	// If we reach the limit signal this as an error!
	res, err := ioutil.ReadAll(io.LimitReader(fd, int64(limit)))
	if len(res) >= limit {
		return nil, Wrap(IOError, "Memory buffer exceeded")
	}

	return res, err
}

func ReadAllWithCtx(
	ctx context.Context,
	scope vfilter.Scope,
	fd io.Reader) ([]byte, error) {

	max_size := int64(10 * 1024 * 1024)

	max_size_any, pres := scope.Resolve(constants.HASH_MAX_SIZE)
	if pres {
		max_size_int, ok := ToInt64(max_size_any)
		if ok {
			max_size = max_size_int
		}
	}

	var result []byte
	buff := pool.Get().(*[]byte)
	defer pool.Put(buff)

	for {
		select {
		case <-ctx.Done():
			return result, nil

		default:
			to_read := max_size - int64(len(result))
			if to_read > int64(len(*buff)) {
				to_read = int64(len(*buff))
			}

			n, err := fd.Read((*buff)[:to_read])
			if err != nil && err != io.EOF {
				return result, err
			}

			if n == 0 {
				return result, nil
			}

			result = append(result, (*buff)[:n]...)
		}
	}
}

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

// An io.Copy() that respects context cancellations.
func CopyWithBuffer(ctx context.Context, dst io.Writer,
	src io.Reader, buff []byte) (n int, err error) {
	offset := 0

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

func CopyN(ctx context.Context, dst io.Writer, src io.Reader, count int64) (
	n int, err error) {
	offset := 0
	buff := pool.Get().(*[]byte)
	defer pool.Put(buff)

	for count > 0 {
		select {
		case <-ctx.Done():
			return offset, nil

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
