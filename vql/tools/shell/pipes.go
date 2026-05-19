package shell

import (
	"context"
	"io"
	"sync"
)

const (
	BUFFER_SIZE = 1024 * 1024
)

func pumpToPipeFromInput(
	ctx context.Context,
	wg *sync.WaitGroup,
	input chan string,
	cb func(message string)) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return

		case in, ok := <-input:
			if !ok {
				return
			}

			cb(in)
		}
	}
}

func pumpFromPipeToOutput(
	ctx context.Context,
	wg *sync.WaitGroup,
	pipe io.Reader,
	cb func(message []byte)) {

	defer wg.Done()

	for {
		buff := make([]byte, BUFFER_SIZE)
		n, err := pipe.Read(buff)
		if err != nil && err != io.EOF {
			if n > 0 {
				cb(buff[:n])
			}
			return
		}

		if n == 0 {
			return
		}

		cb(buff[:n])
	}
}
