package vql

import (
	"encoding/json"

	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	vfilter "www.velocidex.com/golang/vfilter"
)

func ExtractRows(vql_response *actions_proto.VQLResponse) ([]vfilter.Row, error) {
	result := []vfilter.Row{}
	var rows []map[string]interface{}
	err := json.Unmarshal([]byte(vql_response.Response), &rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	for _, row := range rows {
		item := vfilter.NewDict()
		for k, v := range row {
			item.Set(k, v)
		}
		result = append(result, item)
	}

	return result, nil
}

func RowToDict(scope *vfilter.Scope, row vfilter.Row) *vfilter.Dict {
	// If the row is already a dict nothing to do:
	result, ok := row.(*vfilter.Dict)
	if ok {
		return result
	}

	result = vfilter.NewDict()
	for _, column := range scope.GetMembers(row) {
		value, pres := scope.Associative(row, column)
		if pres {
			result.Set(column, value)
		}
	}

	return result
}
