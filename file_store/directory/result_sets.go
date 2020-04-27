package directory

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func GetTimeRange(
	ctx context.Context,
	file_store api.FileStore,
	path_manager api.PathManager,
	start_time, end_time int64) (<-chan *ordereddict.Dict, error) {

	output := make(chan *ordereddict.Dict)

	go func() {
		defer close(output)

		sub_ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for prop := range path_manager.GeneratePaths(sub_ctx) {
			if start_time > 0 && prop.EndTime < start_time {
				continue
			}

			if end_time > 0 && prop.StartTime > end_time {
				return
			}

			row_chan, err := ReadRowsJSON(
				sub_ctx, file_store, prop.Path,
				start_time, end_time)
			if err != nil {
				continue
			}

			for item := range row_chan {
				output <- item
			}
		}

	}()
	return output, nil
}
