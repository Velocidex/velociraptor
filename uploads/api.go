// Uploaders deliver files from accessors to the server (or another target).
package uploads

import (
	"context"
	"io"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/vfilter"
)

// Returned as the result of the query.
type UploadResponse struct {
	Path       string `json:"Path"`
	Size       uint64 `json:"Size"`
	StoredSize uint64 `json:"StoredSize,omitempty"`
	Error      string `json:"Error,omitempty"`
	Sha256     string `json:"sha256,omitempty"`
	Md5        string `json:"md5,omitempty"`
	StoredName string `json:"StoredName,omitempty"`
	Reference  string `json:"Reference,omitempty"`
}

// Provide an uploader capable of uploading any reader object.
type Uploader interface {
	Upload(ctx context.Context,
		scope vfilter.Scope,
		filename *accessors.OSPath,
		accessor string,
		store_as_name string,
		expected_size int64,
		mtime time.Time,
		atime time.Time,
		ctime time.Time,
		btime time.Time,
		reader io.Reader) (*UploadResponse, error)
}

// A generic interface for reporting file ranges. Implementations will
// convert to this common form.

type Range struct {
	// In bytes
	Offset   int64
	Length   int64
	IsSparse bool
}

type RangeReader interface {
	io.Reader
	io.Seeker

	Ranges() []Range
}

func RangeSize(runs []Range) int64 {
	if len(runs) == 0 {
		return 0
	}

	last_run := runs[len(runs)-1]
	return last_run.Offset + last_run.Length
}

// Get the ranges of a reader if possible. If the reader can not
// provide this information we just make a pseudo range that covers
// the entire size of the reader with its expected_size.
func GetRanges(
	ctx context.Context,
	reader io.Reader,
	expected_size int64) (<-chan io.Reader, []Range) {
	output_chan := make(chan io.Reader)

	ranges := []Range{{Length: expected_size}}

	// Can the reader produce ranges?
	range_reader, ok := reader.(RangeReader)
	if ok {
		ranges = range_reader.Ranges()
	}

	go func() {
		defer close(output_chan)

		range_reader, ok := reader.(RangeReader)
		if !ok {
			select {
			case <-ctx.Done():
				return
			case output_chan <- reader:
			}
			return
		}

		for _, rng := range ranges {
			_, err := range_reader.Seek(rng.Offset, io.SeekStart)
			if err == nil {
				select {
				case <-ctx.Done():
					return
				case output_chan <- io.LimitReader(range_reader, rng.Length):
				}
			}
		}

	}()

	return output_chan, ranges
}
