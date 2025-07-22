package result_sets

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type WriteMode bool

const (
	// Constants to improve readability at call sites
	AppendMode   = WriteMode(false)
	TruncateMode = WriteMode(true)
)

type ResultSetWriter interface {
	// Write an already serialized batch of rows. This basically just
	// appends the data to the output JSONL file so it is very cheap.
	WriteJSONL(serialized []byte, total_rows uint64)

	// Alternative writing method
	WriteCompressedJSONL(
		serialized []byte, byte_offset uint64, uncompressed_size int,
		total_rows uint64)

	// Provide a hint as to the next row id we are writing. This is
	// only useful for some implementations of result set writers.
	SetStartRow(start_row int64) error

	// Result sets may be updated in place.
	Update(index uint64, row *ordereddict.Dict) error

	Write(row *ordereddict.Dict)
	Flush()
	Close()

	// Ensures that results are flushed to storage as soon as the
	// writer is closed.
	SetSync()
}

type TimedResultSetWriter interface {
	WriteJSONL(serialized []byte, total_rows int)
	Write(row *ordereddict.Dict)
	Flush()
	Close()
}

type ResultSetReader interface {
	// Returns EOF if the row does not exist in the result set.
	SeekToRow(start int64) error
	Rows(ctx context.Context) <-chan *ordereddict.Dict

	// An alternative method to get the raw json blobs. Avoids having
	// to parse the data from storage.
	JSON(ctx context.Context) (<-chan []byte, error)
	Close()

	TotalRows() int64
	MTime() time.Time
	Stacker() api.FSPathSpec
	SetStacker(stacker api.FSPathSpec)
}

type TimedResultSetReader interface {
	SeekToTime(start time.Time) error
	SetMaxTime(end time.Time)
	Rows(ctx context.Context) <-chan *ordereddict.Dict
	Close()
	GetAvailableFiles(ctx context.Context) []*api.ResultSetFileProperties
}
