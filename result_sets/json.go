package result_sets

import (
	"bufio"
	"context"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/utils"
)

func ReadRowsJSON(
	ctx context.Context,
	config_obj *config_proto.Config,
	log_path string, start_time, end_time int64) (
	<-chan *ordereddict.Dict, error) {

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(log_path)
	if err != nil {
		return nil, err
	}

	output := make(chan *ordereddict.Dict)

	go func() {
		defer close(output)
		defer fd.Close()

		reader := bufio.NewReader(fd)

		for {
			select {
			case <-ctx.Done():
				return

			default:
				row_data, err := reader.ReadBytes('\n')
				if err != nil {
					return
				}
				item := ordereddict.NewDict()
				err = item.UnmarshalJSON(row_data)
				if err != nil {
					return
				}

				ts, pres := item.Get("_ts")
				if pres {
					timestamp, _ := utils.ToInt64(ts)

					if timestamp < start_time {
						continue
					}

					if end_time > 0 && timestamp > end_time {
						return
					}
				}

				output <- item
			}
		}
	}()

	return output, nil
}
