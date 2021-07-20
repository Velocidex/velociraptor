package result_sets

import (
	"context"
	"errors"
	"time"

	"github.com/Velocidex/json"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type ResultSetWriter interface {
	WriteJSONL(serialized []byte, total_rows uint64)
	Write(row *ordereddict.Dict)
	Flush()
	Close()
}

func NewResultSetWriter(
	file_store api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts,
	truncate bool) (ResultSetWriter, error) {
	if rs_factory == nil {
		return nil, errors.New("ResultSet factory not initialized")
	}
	return rs_factory.NewResultSetWriter(
		file_store, path_manager, opts, truncate)
}

type TimedResultSetWriter interface {
	Write(row *ordereddict.Dict)
	Flush()
	Close()
}

func NewTimedResultSetWriter(
	file_store api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts) (TimedResultSetWriter, error) {
	if timed_rs_factory == nil {
		return nil, errors.New("ResultSet factory not initialized")
	}
	return timed_rs_factory.NewTimedResultSetWriter(
		file_store, path_manager, opts)
}

type ResultSetReader interface {
	SeekToRow(start int64) error
	Rows(ctx context.Context) <-chan *ordereddict.Dict
	Close()
	TotalRows() int64
}

func NewResultSetReader(
	file_store api.FileStore,
	path_manager api.PathManager) (ResultSetReader, error) {
	if rs_factory == nil {
		return nil, errors.New("ResultSet factory not initialized")
	}
	return rs_factory.NewResultSetReader(file_store, path_manager)
}

type TimedResultSetReader interface {
	SeekToTime(start time.Time) error
	SetMaxTime(end time.Time)
	Rows(ctx context.Context) <-chan *ordereddict.Dict
	Close()
	GetAvailableFiles(ctx context.Context) []*api.ResultSetFileProperties
}

// Some result sets store events (rows with a timestamp) over periods
// of time. This factory function builds a result set reader over the
// sub result set bounded by the start and end time.
func NewTimedResultSetReader(
	ctx context.Context,
	file_store api.FileStore,
	path_manager api.PathManager) (TimedResultSetReader, error) {
	if timed_rs_factory == nil {
		return nil, errors.New("TimedResultSetReader factory not initialized")
	}
	return timed_rs_factory.NewTimedResultSetReader(
		ctx, file_store, path_manager)
}
