package file_store

import (
	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
)

func PushRows(config_obj *config_proto.Config,
	path_manager api.PathManager,
	rows []*ordereddict.Dict) error {

	file_store_factory := GetFileStore(config_obj)

	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		path_manager, nil, false /* truncate */)
	if err != nil {
		return err
	}

	for _, row := range rows {
		rs_writer.Write(row)
	}

	rs_writer.Close()

	return nil
}
