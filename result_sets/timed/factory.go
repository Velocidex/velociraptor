package timed

import (
	"context"

	"github.com/Velocidex/json"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"

	_ "www.velocidex.com/golang/velociraptor/result_sets/simple"
)

type TimedFactory struct{}

func (self TimedFactory) NewTimedResultSetWriter(
	config_obj *config_proto.Config,
	path_manager api.PathManager,
	opts *json.EncOpts,
	completion func()) (result_sets.TimedResultSetWriter, error) {
	return NewTimedResultSetWriter(
		config_obj, path_manager, opts, completion)
}

func (self TimedFactory) NewTimedResultSetReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	path_manager api.PathManager) (result_sets.TimedResultSetReader, error) {

	return &TimedResultSetReader{
		files:      path_manager.GetAvailableFiles(ctx),
		config_obj: config_obj,
	}, nil
}

func (self TimedFactory) DeleteTimedResultSet(
	ctx context.Context,
	config_obj *config_proto.Config,
	path_manager api.PathManager) error {
	return utils.NotImplementedError
}

func init() {
	result_sets.RegisterTimedResultSetFactory(TimedFactory{})
}
