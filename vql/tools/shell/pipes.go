package shell

import (
	"context"
	"io"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/utils"
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

	buff := make([]byte, BUFFER_SIZE)
	for !utils.IsCtxDone(ctx) {
		n, err := pipe.Read(buff)
		if err != nil { // EOF on pipe close.
			if n > 0 {
				cb(buff[:n])
			}
			return
		}

		// If the buffer is not full, wait for it be filled. Sleep a
		// small amount to allow the pipe to fill up so we don't end up
		// making lots of small reads and generating many small rows.
		if n < BUFFER_SIZE {
			time.Sleep(300 * time.Millisecond)
		}

		cb(buff[:n])
	}
}
