// Based on https://github.com/orcaman/writerseeker

package file_store

import (
	"bytes"
	"errors"
	"io"
)

// WriterSeeker is an in-memory io.WriteSeeker implementation
type WriterSeeker struct {
	buf []byte
	pos int
}

// Write writes to the buffer of this WriterSeeker instance
func (ws *WriterSeeker) Write(p []byte) (n int, err error) {
	minCap := ws.pos + len(p)
	if minCap > cap(ws.buf) { // Make sure buf has enough capacity:
		buf2 := make([]byte, len(ws.buf), minCap+len(p)) // add some extra
		copy(buf2, ws.buf)
		ws.buf = buf2
	}
	if minCap > len(ws.buf) {
		ws.buf = ws.buf[:minCap]
	}
	copy(ws.buf[ws.pos:], p)
	ws.pos += len(p)
	return len(p), nil
}

// Seek seeks in the buffer of this WriterSeeker instance
func (ws *WriterSeeker) Seek(offset int64, whence int) (int64, error) {
	newPos, offs := 0, int(offset)
	switch whence {
	case io.SeekStart:
		newPos = offs
	case io.SeekCurrent:
		newPos = ws.pos + offs
	case io.SeekEnd:
		newPos = len(ws.buf) + offs
	}
	if newPos < 0 {
		return 0, errors.New("negative result pos")
	}
	ws.pos = newPos
	return int64(newPos), nil
}

// Reader returns an io.Reader. Use it, for example, with io.Copy, to copy the content of the WriterSeeker buffer to an io.Writer
func (ws *WriterSeeker) Reader() io.Reader {
	return bytes.NewReader(ws.buf)
}

// Close :
func (ws *WriterSeeker) Close() error {
	return nil
}

// BytesReader returns a *bytes.Reader. Use it when you need a reader that implements the io.ReadSeeker interface
func (ws *WriterSeeker) BytesReader() *bytes.Reader {
	return bytes.NewReader(ws.buf)
}
