package api

import (
	context "golang.org/x/net/context"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/vfilter"
)

func RunVQL(
	ctx context.Context,
	config_obj *api_proto.Config,
	env *vfilter.Dict,
	query string) (*api_proto.GetTableResponse, error) {

	result := &api_proto.GetTableResponse{}

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	env.Set("server_config", config_obj)

	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = logging.NewPlainLogger(config_obj,
		&logging.ToolComponent)

	vql, err := vfilter.Parse(query)
	if err != nil {
		return nil, err
	}

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for row := range vql.Eval(sub_ctx, scope) {
		if len(result.Columns) == 0 {
			result.Columns = scope.GetMembers(row)
		}

		new_row := &api_proto.Row{}
		for _, column := range result.Columns {
			value, pres := scope.Associative(row, column)
			if !pres {
				value = ""
			}
			new_row.Cell = append(new_row.Cell, csv.AnyToString(value))
		}

		result.Rows = append(result.Rows, new_row)
	}

	return result, nil
}
