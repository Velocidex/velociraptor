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
