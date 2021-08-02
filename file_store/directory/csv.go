package directory

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/utils"
)

func row_to_dict(row_data []interface{}, headers []string) (*ordereddict.Dict, int64) {
	row := ordereddict.NewDict()
	var timestamp int64

	for idx, row_item := range row_data {
		if idx > len(headers) {
			break
		}

		// Event logs have a _ts column representing the time
		// of each event.
		column_name := headers[idx]
		if column_name == "_ts" {
			timestamp, _ = utils.ToInt64(row_item)
		}

		row.Set(column_name, row_item)
	}

	return row, timestamp
}

func ReadRowsCSV(
	ctx context.Context,
	file_store api.FileStore,
	log_path api.SafeDatastorePath, start_time, end_time int64) (
	<-chan *ordereddict.Dict, error) {

	fd, err := file_store.ReadFile(log_path)
	if err != nil {
		return nil, err
	}

	// Read the headers which are the first row.
	csv_reader := csv.NewReader(fd)
	headers, err := csv_reader.Read()
	if err != nil {
		return nil, err
	}

	output := make(chan *ordereddict.Dict)

	go func() {
		defer close(output)
		defer fd.Close()

		for {
			select {
			case <-ctx.Done():
				return

			default:
				row_data, err := csv_reader.ReadAny()
				if err != nil {
					return
				}

				dict, timestamp := row_to_dict(row_data, headers)
				if timestamp < start_time {
					continue
				}

				if end_time > 0 && timestamp > end_time {
					return
				}

				output <- dict
			}
		}
	}()

	return output, nil
}
