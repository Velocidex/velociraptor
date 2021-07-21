package directory

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func ReadRows(
	ctx context.Context,
	file_store api.FileStore,
	file_path string,
	start_time, end_time int64) (<-chan *ordereddict.Dict, error) {

	// Backwards compatibility: We dont write .csv files any more
	// but we can read them if they are there.
	if strings.HasSuffix(file_path, ".csv") {
		return ReadRowsCSV(ctx, file_store, file_path, start_time, end_time)
	}

	// If we are supposed to read a json file but we cant find it
	// - maybe it is a csv file instead - look for it.
	row_chan, err := ReadRowsJSON(ctx, file_store, file_path, start_time, end_time)
	if err != nil && strings.HasSuffix(file_path, ".json") {
		file_path = strings.TrimSuffix(file_path, ".json") + ".csv"
		return ReadRowsCSV(ctx, file_store, file_path, start_time, end_time)
	}

	return row_chan, err
}
