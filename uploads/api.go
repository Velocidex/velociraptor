// Uploaders deliver files from accessors to the server (or another target).
package uploads

import (
	"io"
)

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
// provide this information we just make a psuedo range.
func GetRanges(reader io.Reader, expected_size int64) (<-chan io.Reader, []Range) {
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
			output_chan <- reader
			return
		}

		for _, rng := range ranges {
			_, err := range_reader.Seek(rng.Offset, io.SeekStart)
			if err == nil {
				output_chan <- io.LimitReader(range_reader, rng.Length)
			}
		}

	}()

	return output_chan, ranges
}
