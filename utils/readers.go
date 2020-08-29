package utils

import (
	"io"

	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
)

type ReaderAtter struct {
	Reader io.ReadSeeker
}

func (self ReaderAtter) ReadAt(buf []byte, offset int64) (int, error) {
	_, err := self.Reader.Seek(offset, 0)
	if err != nil {
		return 0, err
	}
	return self.Reader.Read(buf)
}

type BufferReaderAt struct {
	Buffer []byte
}

func (self *BufferReaderAt) ReadAt(buf []byte, offset int64) (int, error) {
	to_read := int64(len(buf))
	if offset < 0 {
		to_read += offset
		offset = 0
	}

	if offset+to_read > int64(len(self.Buffer)) {
		to_read = int64(len(self.Buffer)) - offset
	}

	if to_read < 0 {
		return 0, nil
	}

	n := copy(buf, self.Buffer[offset:offset+to_read])

	return n, nil
}

// A Reader that accepts an index. Velociraptor stores sparse files in
// the data store using a regular file and an index file. The regular
// file is simply data runs stored back to back with no gaps. The
// index maps between the original file offsets (which might include
// gaps) to the flat file offsets (which have gaps removed).
type RangedReader struct {
	io.ReaderAt

	Index *actions_proto.Index
}

// Here file_offset refers to the original sparse file on the client.
func (self *RangedReader) ReadAt(buf []byte, file_offset int64) (
	int, error) {
	if self.Index == nil {
		return 0, errors.New("RangedReader: No index")
	}

	buf_idx := 0

	// Find the run which covers the required offset.
	for j := 0; j < len(self.Index.Ranges) && buf_idx < len(buf); j++ {
		run := self.Index.Ranges[j]

		// This run can provide us with some data.
		if run.OriginalOffset <= file_offset &&
			file_offset < run.OriginalOffset+run.Length {

			// The relative offset within the run.
			run_offset := int(file_offset - run.OriginalOffset)

			n, err := self.readFromARun(j, buf[buf_idx:], run_offset)
			if err != nil {
				return buf_idx, err
			}

			if n == 0 {
				return buf_idx, io.EOF
			}

			buf_idx += n
			file_offset += int64(n)
		}
	}

	if buf_idx == 0 {
		return 0, io.EOF
	}

	return buf_idx, nil
}

func (self *RangedReader) readFromARun(
	run_idx int, buf []byte,
	// The offset within the run to read from.
	run_offset int) (int, error) {

	// Read from the run as much data as is available.
	run := self.Index.Ranges[run_idx]

	// The run is sparse since there is no data in the file.
	if run.FileLength == 0 {
		to_read := run.Length - int64(run_offset)
		if to_read > int64(len(buf)) {
			to_read = int64(len(buf))
		}

		for i := int64(0); i < to_read; i++ {
			buf[i] = 0
		}
		return int(to_read), nil
	}

	if run.FileLength > 0 {
		to_read := run.FileLength - int64(run_offset)
		if int64(len(buf)) < to_read {
			to_read = int64(len(buf))
		}

		if to_read == 0 {
			return 0, io.EOF
		}

		// Run contains data - read it into the buffer using
		// the embedded reader.
		return self.ReaderAt.ReadAt(
			buf[:to_read], run.FileOffset+int64(run_offset))
	}

	return 0, errors.New("IO Error")
}
