package mscfb

import (
	"fmt"
	"io"
	"runtime/debug"
	"sync"

	"www.velocidex.com/golang/velociraptor/accessors"
)

type readAdapter struct {
	sync.Mutex

	info   accessors.FileInfo
	pos    int64
	reader io.ReaderAt
}

func (self *readAdapter) Read(buf []byte) (res int, err error) {
	self.Lock()
	defer self.Unlock()

	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			debug.PrintStack()
			err, _ = r.(error)
		}
	}()

	res, err = self.reader.ReadAt(buf, self.pos)

	// If ReadAt is unable to read anything it means an EOF.
	if res == 0 {
		// The NTFS cache may be flushed during this read and in this
		// case the file handle will be closed on us during the
		// read. This usually shows up as an EOF read with 0 length.
		// See Issue
		// https://github.com/Velocidex/velociraptor/issues/2153

		// We catch this issue by issuing one more read just to make
		// sure. Usually we are wrapping a ReadAtter here and we do
		// not expect to see a EOF anyway. In the case of NTFS the
		// extra read will re-open the underlying device file with a
		// new NTFS context (reparsing the $MFT and purging all the
		// caches) so the next read will succeed.
		res, err = self.reader.ReadAt(buf, self.pos)
		if res == 0 {
			// Still EOF - give up
			return res, io.EOF
		}
	}

	self.pos += int64(res)

	return res, err
}

func (self *readAdapter) ReadAt(buf []byte, offset int64) (int, error) {
	self.Lock()
	defer self.Unlock()
	self.pos = offset

	return self.reader.ReadAt(buf, offset)
}

func (self *readAdapter) Close() error {
	return nil
}

func (self *readAdapter) Seek(offset int64, whence int) (int64, error) {
	self.Lock()
	defer self.Unlock()

	self.pos = offset
	return self.pos, nil
}
