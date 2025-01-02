package tables

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func getNotebookTable(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest,
	principal string) (*api_proto.GetTableResponse, error) {

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		return nil, err
	}

	_, err = notebook_manager.GetSharedNotebooks(ctx, principal)
	if err != nil {
		return nil, err
	}

	return getTable(ctx, config_obj, in, principal)
}
