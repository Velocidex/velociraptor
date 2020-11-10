package result_sets

import (
	"context"

	"github.com/Velocidex/json"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	factory Factory
)

type Factory interface {
	NewResultSetWriter(
		file_store_factory api.FileStore,
		path_manager api.PathManager,
		opts *json.EncOpts,
		truncate bool) (ResultSetWriter, error)

	NewResultSetReader(
		file_store_factory api.FileStore,
		path_manager api.PathManager) (ResultSetReader, error)

	NewTimedResultSetReader(
		ctx context.Context,
		file_store api.FileStore,
		path_manager api.PathManager,
		start_time, end_time uint64) (ResultSetReader, error)
}

// Allows for registration of the result set factory.
func Register(impl Factory) {
	factory = impl
}
