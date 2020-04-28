// A Data store specific way of storing and managing result sets.

// Velociraptor is essentially a query engine, therefore all the
// results are stored in result sets. This interface abstracts away
// where and how result sets are stored.

package file_store

import (
	"context"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
)

// GetTimeRange returns a channel thats feeds results from the
// filestore to the caller. Where and how the results are actually
// stored is abstracted and depends on the filestore implementation.
func GetTimeRange(
	ctx context.Context,
	config_obj *config_proto.Config,
	path_manager api.PathManager,
	start_time, end_time int64) (<-chan *ordereddict.Dict, error) {
	file_store_factory := GetFileStore(config_obj)

	switch config_obj.Datastore.Implementation {
	default:
		return directory.GetTimeRange(
			ctx, file_store_factory, path_manager, start_time, end_time)
	}
}
