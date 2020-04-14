package clients

import (
	"strings"

	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
)

func LabelClients(
	config_obj *config_proto.Config,
	in *api_proto.LabelClientsRequest) (*api_proto.APIResponse, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	index_func := db.SetIndex
	switch in.Operation {
	case "remove":
		index_func = db.UnsetIndex
	case "check":
		index_func = db.CheckIndex
	case "set":
		// default.
	default:
		return nil, errors.New(
			"unknown label operation. Must be set, check or remove")
	}

	for _, label := range in.Labels {
		for _, client_id := range in.ClientIds {
			if !strings.HasPrefix(label, "label:") {
				label = "label:" + label
			}
			err = index_func(
				config_obj,
				constants.CLIENT_INDEX_URN,
				client_id, []string{label})
			if err != nil {
				return nil, err
			}
			err = index_func(
				config_obj,
				constants.CLIENT_INDEX_URN,
				label, []string{client_id})
			if err != nil {
				return nil, err
			}
		}
	}

	return &api_proto.APIResponse{}, nil
}
