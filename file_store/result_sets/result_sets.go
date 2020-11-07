package result_sets

import (
	"context"
	"errors"

	"github.com/Velocidex/json"
	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type ResultSetWriter interface {
	WriteJSONL(serialized []byte, total_rows uint64)
	Write(row *ordereddict.Dict)
	Flush()
	Close()
}

func NewResultSetWriter(
	config_obj *config_proto.Config,
	path_manager api.PathManager,
	opts *json.EncOpts,
	truncate bool) (ResultSetWriter, error) {
	if factory == nil {
		return nil, errors.New("ResultSet factory not initialized")
	}
	return factory.NewResultSetWriter(config_obj, path_manager, opts, truncate)
}

type ResultSetReader interface {
	SeekToRow(start int64) error
	Rows(ctx context.Context) <-chan *ordereddict.Dict
	Close()
	TotalRows() int64
}

func NewResultSetReader(
	config_obj *config_proto.Config,
	path_manager api.PathManager) (ResultSetReader, error) {
	if factory == nil {
		return nil, errors.New("ResultSet factory not initialized")
	}
	return factory.NewResultSetReader(config_obj, path_manager)
}
