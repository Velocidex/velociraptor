package result_sets

import (
	"context"
	"errors"

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
	if factory == nil {
		return nil, errors.New("ResultSet factory not initialized")
	}
	return factory.NewResultSetWriter(file_store, path_manager, opts, truncate)
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
	if factory == nil {
		return nil, errors.New("ResultSet factory not initialized")
	}
	return factory.NewResultSetReader(file_store, path_manager)
}
