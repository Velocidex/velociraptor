package tables

import (
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

func ConvertTimelineRowsToTableResponse(
	in <-chan *ordereddict.Dict,
	result *api_proto.GetTableResponse,
	timezone string,
	limit uint64,
) *api_proto.GetTableResponse {
	opts := json.GetJsonOptsForTimezone(timezone)

	var rows uint64
	column_known := make(map[string]bool)
	for row := range in {
		// Row has timestamp
		timestamp_any, pres := row.Get("Timestamp")
		if !pres {
			continue
		}

		timestamp, ok := timestamp_any.(time.Time)
		if !ok {
			continue
		}

		if result.StartTime == 0 {
			result.StartTime = timestamp.UnixNano()
		}
		result.EndTime = timestamp.UnixNano()

		data := make(map[string]string)
		for _, key := range row.Keys() {
			// Do we already know about this column?
			_, pres := column_known[key]
			if !pres {
				result.Columns = append(result.Columns, key)
				column_known[key] = true
			}

			value, pres := row.Get(key)
			if pres {
				data[key] = json.AnyToString(value, opts)
			} else {
				data[key] = "null"
			}
		}

		row_proto := &api_proto.Row{}
		for _, k := range result.Columns {
			value, pres := data[k]
			if !pres {
				value = "null"
			}
			row_proto.Cell = append(row_proto.Cell, value)
		}
		result.Rows = append(result.Rows, row_proto)

		rows += 1
		if rows >= limit {
			break
		}
	}

	return result
}
