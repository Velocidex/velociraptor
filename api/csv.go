package api

import (
	"io"

	"www.velocidex.com/golang/velociraptor/file_store/csv"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

func getTable(config_obj *api_proto.Config, in *api_proto.GetTableRequest) (
	*api_proto.GetTableResponse, error) {

	rows := uint64(0)
	if in.Rows == 0 {
		in.Rows = 500
	}

	fd, err := getFileForVFSPath(config_obj, in.ClientId, in.Path)
	if err != nil {
		return nil, err
	}

	csv_reader := csv.NewReader(fd)
	headers, err := csv_reader.Read()
	if err != nil {
		if err == io.EOF {
			return &api_proto.GetTableResponse{}, nil
		}
		return nil, err
	}

	result := &api_proto.GetTableResponse{
		Columns: headers,
	}

	for {
		row_data, err := csv_reader.Read()
		if err != nil {
			break
		}
		result.Rows = append(result.Rows, &api_proto.Row{
			Cell: row_data,
		})

		rows += 1
		if rows > in.Rows {
			break
		}
	}

	return result, nil
}
