package directory

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func ReadRows(
	ctx context.Context,
	file_store api.FileStore,
	file_path api.FSPathSpec,
	start_time, end_time int64) (<-chan *ordereddict.Dict, error) {

	return ReadRowsJSON(ctx, file_store, file_path, start_time, end_time)
}
