package result_sets

import (
	"context"
	"errors"

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

func NewTimedResultSetWriter(
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts) (TimedResultSetWriter, error) {
	if timed_rs_factory == nil {
		panic(errors.New("TimedFactory not initialized"))
	}
	return timed_rs_factory.NewTimedResultSetWriter(file_store_factory,
		path_manager, opts)
}

func NewTimedResultSetReader(
	ctx context.Context,
	file_store_factory api.FileStore,
	path_manager api.PathManager) (TimedResultSetReader, error) {
	if timed_rs_factory == nil {
		panic(errors.New("TimedFactory not initialized"))
	}
	return timed_rs_factory.NewTimedResultSetReader(ctx,
		file_store_factory, path_manager)
}

type Factory interface {
	NewResultSetWriter(
		file_store_factory api.FileStore,
		log_path api.PathSpec,
		opts *json.EncOpts,
		truncate bool) (ResultSetWriter, error)

	NewResultSetReader(
		file_store_factory api.FileStore,
		log_path api.PathSpec,
	) (ResultSetReader, error)
}

func NewResultSetWriter(
	file_store_factory api.FileStore,
	log_path api.PathSpec,
	opts *json.EncOpts,
	truncate bool) (ResultSetWriter, error) {
	if rs_factory == nil {
		panic(errors.New("ResultSetFactory not initialized"))
	}
	return rs_factory.NewResultSetWriter(file_store_factory,
		log_path, opts, truncate)

}

func NewResultSetReader(
	file_store_factory api.FileStore,
	log_path api.PathSpec) (ResultSetReader, error) {
	if rs_factory == nil {
		panic(errors.New("ResultSetFactory not initialized"))
	}
	return rs_factory.NewResultSetReader(file_store_factory, log_path)
}

// Allows for registration of the result set factory.
func RegisterResultSetFactory(impl Factory) {
	rs_factory = impl
}

func RegisterTimedResultSetFactory(impl TimedFactory) {
	timed_rs_factory = impl
}
