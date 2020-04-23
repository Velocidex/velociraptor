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
	result := &api_proto.APIResponse{}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	if in.Operation == "check" {
		for _, label := range in.Labels {
			for _, client_id := range in.ClientIds {
				if !strings.HasPrefix(label, "label:") {
					label = "label:" + label
				}
				err = db.CheckIndex(
					config_obj,
					constants.CLIENT_INDEX_URN,
					client_id, []string{label})
				if err == nil {
					return result, nil
				}

				err = db.CheckIndex(
					config_obj,
					constants.CLIENT_INDEX_URN,
					label, []string{client_id})
				if err == nil {
					return result, nil
				}

				err = db.CheckIndex(
					config_obj,
					constants.CLIENT_INDEX_URN,
					"__"+strings.ToLower(label),
					[]string{client_id})
				if err == nil {
					return result, nil
				}
			}
		}
	}

	index_func := db.SetIndex
	switch in.Operation {
	case "remove":
		index_func = db.UnsetIndex

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
			err = index_func(
				config_obj,
				constants.CLIENT_INDEX_URN,
				"__label:"+strings.ToLower(label),
				[]string{client_id})
			if err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}
