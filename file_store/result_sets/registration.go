package result_sets

import (
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
}

// Allows for registration of the result set factory.
func Register(impl Factory) {
	factory = impl
}
