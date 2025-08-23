package tables

import (
	"context"
	"time"

	errors "github.com/go-errors/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

func getTimeline(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	if in.NotebookId == "" {
		return nil, errors.New("NotebookId must be specified")
	}

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		return nil, err
	}

	options := services.TimelineOptions{
		IncludeComponents: in.IncludeComponents,
		ExcludeComponents: in.SkipComponents}

	if in.StartTime != 0 {
		options.StartTime = time.Unix(0, int64(in.StartTime))
	}

	if in.FilterRegex != "" {
		options.Filter = in.FilterRegex
	}

	reader, err := notebook_manager.ReadTimeline(ctx, in.NotebookId,
		in.Timeline, options)
	if err != nil {
		return nil, err
	}

	result := &api_proto.GetTableResponse{
		Timelines: reader.Stat().Timelines,
	}
	return ConvertTimelineRowsToTableResponse(
		ctx, reader, result, in.Timezone, in.Rows), nil
}

func ConvertTimelineRowsToTableResponse(
	ctx context.Context,
	reader services.TimelineReader,
	result *api_proto.GetTableResponse,
	timezone string,
	limit uint64,
) *api_proto.GetTableResponse {
	opts := json.GetJsonOptsForTimezone(timezone)

	var rows uint64
	column_known := make(map[string]bool)
	for row := range reader.Read(ctx) {
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

		data := make(map[string]interface{})
		for _, i := range row.Items() {
			// Do we already know about this column?
			_, pres := column_known[i.Key]
			if !pres {
				result.Columns = append(result.Columns, i.Key)
				column_known[i.Key] = true
			}

			data[i.Key] = i.Value
		}

		json_out := make([]interface{}, 0, len(result.Columns))
		for _, k := range result.Columns {
			value, _ := data[k]
			json_out = append(json_out, value)
		}
		serialized, err := json.MarshalWithOptions(json_out, opts)
		if err != nil {
			continue
		}

		result.Rows = append(result.Rows, &api_proto.Row{
			Json: string(serialized),
		})

		rows += 1
		if rows >= limit {
			break
		}
	}

	return result
}
