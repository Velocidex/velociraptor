package result_sets

import (
	"github.com/Velocidex/json"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	factory Factory
)

type Factory interface {
	NewResultSetWriter(
		config_obj *config_proto.Config,
		path_manager api.PathManager,
		opts *json.EncOpts,
		truncate bool) (ResultSetWriter, error)

	NewResultSetReader(
		config_obj *config_proto.Config,
		path_manager api.PathManager) (ResultSetReader, error)
}

// Allows for registration of the result set factory.
func Register(impl Factory) {
	factory = impl
}
