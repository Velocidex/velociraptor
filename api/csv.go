package api

import (
	"io"
	"path"
	"strings"

	"www.velocidex.com/golang/velociraptor/file_store/csv"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
)

func getTable(config_obj *api_proto.Config, in *api_proto.GetTableRequest) (
	*api_proto.GetTableResponse, error) {

	rows := uint64(0)
	if in.Rows == 0 {
		in.Rows = 500
	}

	file_path := ""
	if in.ClientId == "" && strings.HasPrefix(in.Path, "hunts") {
		file_path = in.Path
	} else {
		file_path = path.Join("clients", in.ClientId, in.Path)
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(file_path)
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
