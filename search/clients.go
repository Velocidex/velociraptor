package search

import (
	"context"
	"errors"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

func GetApiClient(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, detailed bool) (
	*api_proto.ApiClient, error) {

	if config_obj.GUI == nil {
		return nil, errors.New("GUI not configured")
	}

	result := &api_proto.ApiClient{
		ClientId: client_id,
	}

	// Special well known client id.
	if client_id == "server" {
		return result, nil
	}

	if client_id == "" || client_id[0] != 'C' {
		return nil, errors.New("client_id must start with C")
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	labeler := services.GetLabeler()
	if labeler == nil {
		return nil, errors.New("Labeler not ready")
	}

	result.Labels = labeler.GetClientLabels(config_obj, client_id)

	client_info := &actions_proto.ClientInfo{}
	err = db.GetSubject(config_obj, client_path_manager.Path(), client_info)
	if err != nil {
		return nil, err
	}

	result.LastInterrogateFlowId = client_info.LastInterrogateFlowId
	result.AgentInformation = &api_proto.AgentInformation{
		Version: client_info.ClientVersion,
		Name:    client_info.ClientName,
	}

	result.OsInfo = &api_proto.Uname{
		System:   client_info.System,
		Hostname: client_info.Hostname,
		Release:  client_info.Release,
		Machine:  client_info.Architecture,
		Fqdn:     client_info.Fqdn,
	}

	public_key_info := &crypto_proto.PublicKey{}
	err = db.GetSubject(config_obj, client_path_manager.Key(),
		public_key_info)
	if err != nil {
		// Offline clients do not have public key files, so
		// this is not actually an error.
	}

	result.FirstSeenAt = public_key_info.EnrollTime

	err = db.GetSubject(config_obj, client_path_manager.Ping(),
		client_info)
	if err != nil {
		// Offline clients do not have ping files, so
		// this is not actually an error.
	}

	result.LastSeenAt = client_info.Ping
	result.LastIp = client_info.IpAddress

	return result, nil
}

// A fast way of getting some client information. Reduces reads from
// the backend by caching as much data as possible - may not be the
// most up to date information but does contain the latest ping
// data. This is mostly used to display the results from the search
// screen.
func FastGetApiClient(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) (*api_proto.ApiClient, error) {

	client_info_manager, err := services.GetClientInfoManager()
	if err != nil {
		return nil, err
	}

	client_info, err := client_info_manager.Get(client_id)
	if err != nil {
		return nil, err
	}

	if client_info == nil {
		return nil, errors.New("Invalid client_info")
	}

	labeler := services.GetLabeler()
	if labeler == nil {
		return nil, errors.New("Labeler not ready")
	}

	return &api_proto.ApiClient{
		ClientId: client_id,
		Labels:   labeler.GetClientLabels(config_obj, client_id),
		AgentInformation: &api_proto.AgentInformation{
			Version: client_info.ClientVersion,
			Name:    client_info.ClientName,
		},
		OsInfo: &api_proto.Uname{
			System:   client_info.System,
			Hostname: client_info.Hostname,
			Release:  client_info.Release,
			Machine:  client_info.Architecture,
			Fqdn:     client_info.Fqdn,
		},
		LastSeenAt:            client_info.Ping,
		LastIp:                client_info.IpAddress,
		LastInterrogateFlowId: client_info.LastInterrogateFlowId,
	}, nil
}
