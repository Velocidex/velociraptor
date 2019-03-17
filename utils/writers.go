package utils

import "io"

type teeWriter struct {
	writers []io.Writer
}

func (self *teeWriter) Write(p []byte) (n int, err error) {
	for _, writer := range self.writers {
		n, err := writer.Write(p)
		if err != nil {
			return n, err
		}
	}

	return len(p), nil
}

// MultiWriter creates a writer that duplicates its writes to all the
// provided writers, similar to the Unix tee(1) command.
func NewTee(writers ...io.Writer) io.Writer {
	return &teeWriter{
		writers: writers,
	}
}
