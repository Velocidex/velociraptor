package api

import (
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/datastore"
)

func GetApiClient(
	config_obj *config.Config, client_id string, detailed bool) (
	*api_proto.ApiClient, error) {
	result := &api_proto.ApiClient{
		ClientId: client_id,
	}

	client_urn := "aff4:/" + client_id
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	client_info := &actions_proto.ClientInfo{}
	err = db.GetSubject(config_obj, client_urn, client_info)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if detailed {
		result.Info = client_info.Info
	}

	result.AgentInformation = &api_proto.AgentInformation{
		Version: client_info.ClientVersion,
		Name:    client_info.ClientName,
	}

	result.OsInfo = &actions_proto.Uname{
		System:  client_info.System,
		Release: client_info.Release,
		Machine: client_info.Architecture,
		Fqdn:    client_info.Fqdn,
	}

	if client_info.Knowledgebase != nil {
		for _, user := range client_info.Knowledgebase.Users {
			result.Users = append(result.Users, user)
		}
	}

	return result, nil
}
