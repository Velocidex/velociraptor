package result_sets

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type ResultSetWriter interface {
	WriteJSONL(serialized []byte, total_rows uint64)
	Write(row *ordereddict.Dict)
	Flush()
	Close()
}

type TimedResultSetWriter interface {
	Write(row *ordereddict.Dict)
	Flush()
	Close()
}

type ResultSetReader interface {
	SeekToRow(start int64) error
	Rows(ctx context.Context) <-chan *ordereddict.Dict
	Close()
	TotalRows() int64
}

type TimedResultSetReader interface {
	SeekToTime(start time.Time) error
	SetMaxTime(end time.Time)
	Rows(ctx context.Context) <-chan *ordereddict.Dict
	Close()
	GetAvailableFiles(ctx context.Context) []*api.ResultSetFileProperties
}
