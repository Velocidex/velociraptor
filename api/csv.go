package api

import (
	"path"

	"www.velocidex.com/golang/velociraptor/file_store/csv"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
)

func getTable(config_obj *api_proto.Config, in *api_proto.GetTableRequest) (
	*api_proto.GetTableResponse, error) {

	file_path := path.Join("clients", in.ClientId, in.Path)
	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(file_path)
	if err != nil {
		return nil, err
	}

	csv_reader := csv.NewReader(fd)
	headers, err := csv_reader.Read()
	if err != nil {
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
	}

	return result, nil
}
