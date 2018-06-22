package api

import (
	"errors"
	"github.com/golang/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
)

func GetApiClient(config_obj *config.Config, client_id string) (
	*api_proto.ApiClient, error) {
	result := &api_proto.ApiClient{
		ClientId: client_id,
	}

	client_urn := "aff4:/" + client_id
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	serialized_client_info, pres := db.GetSubjectAttribute(
		config_obj, client_urn, constants.CLIENT_VELOCIRAPTOR_INFO)
	if !pres {
		return nil, errors.New("Not found")
	}

	client_info := &actions_proto.ClientInfo{}
	err = proto.Unmarshal(serialized_client_info, client_info)
	if err != nil {
		return nil, err
	}

	result.AgentInformation = &api_proto.AgentInformation{
		Version: client_info.ClientVersion,
		Name:    client_info.ClientName,
	}

	result.OsInfo = &api_proto.Uname{
		System:  client_info.System,
		Release: client_info.Release,
		Machine: client_info.Architecture,
		Fqdn:    client_info.Fqdn,
	}

	for _, user := range client_info.Knowledgebase.Users {
		result.Users = append(result.Users, user)
	}

	return result, nil
}
