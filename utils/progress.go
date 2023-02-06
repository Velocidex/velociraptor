package utils

import (
	"context"
	"io"
	"sync"
	"time"
)

type ProgressReporter interface {
	Report(progress string)
}

type DurationProgressWriter struct {
	cb      func(byte_count int, d time.Duration)
	mu      sync.Mutex
	count   int
	started time.Time
	writer  io.Writer
}

func (self *DurationProgressWriter) Write(buf []byte) (int, error) {
	n, err := self.writer.Write(buf)
	self.mu.Lock()
	self.count += n
	self.mu.Unlock()
	return n, err
}

func NewDurationProgressWriter(
	ctx context.Context, cb func(byte_count int, d time.Duration),
	writer io.Writer, period time.Duration) (*DurationProgressWriter, func()) {
	self := &DurationProgressWriter{
		cb:      cb,
		started: GetTime().Now(),
		writer:  writer,
	}

	sub_ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer cancel()

		for {
			select {
			case <-sub_ctx.Done():
				return

			case <-time.After(period):
				self.mu.Lock()
				duration := GetTime().Now().Sub(self.started)
				cb(self.count, duration)
				self.mu.Unlock()
			}
		}
	}()

	return self, cancel
}
