package utils

import (
	"io"
)

type TeeWriter struct {
	writers []io.Writer
	count   int
}

func (self *TeeWriter) Count() int {
	return self.count
}

func (self *TeeWriter) Write(p []byte) (n int, err error) {
	for _, writer := range self.writers {
		n, err = writer.Write(p)
		self.count += n
		if err != nil && err != io.EOF {
			return n, err
		}
	}

	return len(p), nil
}

// MultiWriter creates a writer that duplicates its writes to all the
// provided writers, similar to the Unix tee(1) command.
func NewTee(writers ...io.Writer) *TeeWriter {
	return &TeeWriter{
		writers: writers,
	}
}
