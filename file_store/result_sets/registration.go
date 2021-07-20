package result_sets

import (
	"context"

	"github.com/Velocidex/json"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	rs_factory       Factory
	timed_rs_factory TimedFactory
)

type TimedFactory interface {
	NewTimedResultSetWriter(
		file_store_factory api.FileStore,
		path_manager api.PathManager,
		opts *json.EncOpts) (TimedResultSetWriter, error)

	NewTimedResultSetReader(
		ctx context.Context,
		file_store api.FileStore,
		path_manager api.PathManager) (TimedResultSetReader, error)
}

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
func RegisterResultSetFactory(impl Factory) {
	rs_factory = impl
}

func RegisterTimedResultSetFactory(impl TimedFactory) {
	timed_rs_factory = impl
}
