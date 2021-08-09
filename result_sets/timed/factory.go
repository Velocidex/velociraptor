package timed

import (
	"context"

	"github.com/Velocidex/json"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"

	_ "www.velocidex.com/golang/velociraptor/result_sets/simple"
)

type TimedFactory struct{}

func (self TimedFactory) NewTimedResultSetWriter(
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts) (result_sets.TimedResultSetWriter, error) {
	return NewTimedResultSetWriter(
		file_store_factory, path_manager, opts)
}

func (self TimedFactory) NewTimedResultSetWriterWithClock(
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts, clock utils.Clock) (result_sets.TimedResultSetWriter, error) {
	return NewTimedResultSetWriterWithClock(
		file_store_factory, path_manager, opts, clock)
}

func (self TimedFactory) NewTimedResultSetReader(
	ctx context.Context,
	file_store_factory api.FileStore,
	path_manager api.PathManager) (result_sets.TimedResultSetReader, error) {

	return &TimedResultSetReader{
		files:              path_manager.GetAvailableFiles(ctx),
		file_store_factory: file_store_factory,
	}, nil
}

func init() {
	result_sets.RegisterTimedResultSetFactory(TimedFactory{})
}
