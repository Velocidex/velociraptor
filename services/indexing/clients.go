package indexing

import (
	"context"
	"errors"

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

	labeler := services.GetLabeler(config_obj)
	if labeler == nil {
		return nil, errors.New("Labeler not ready")
	}

	result.Labels = labeler.GetClientLabels(ctx, config_obj, client_id)
	result.LastLabelTimestamp = labeler.LastLabelTimestamp(ctx, config_obj, client_id)

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	client_info, err := client_info_manager.Get(ctx, client_id)
	if err != nil {
		return nil, err
	}

	result.LastInterrogateFlowId = client_info.LastInterrogateFlowId
	result.LastInterrogateArtifactName = client_info.LastInterrogateArtifactName
	result.AgentInformation = &api_proto.AgentInformation{
		Version:   client_info.ClientVersion,
		BuildTime: client_info.BuildTime,
		Name:      client_info.ClientName,
	}

	result.OsInfo = &api_proto.Uname{
		System:       client_info.System,
		Hostname:     client_info.Hostname,
		Release:      client_info.Release,
		Machine:      client_info.Architecture,
		Fqdn:         client_info.Fqdn,
		MacAddresses: client_info.MacAddresses,
	}

	public_key_info := &crypto_proto.PublicKey{}
	err = db.GetSubject(config_obj, client_path_manager.Key(),
		public_key_info)
	if err != nil {
		// Offline clients do not have public key files, so
		// this is not actually an error.
	}

	result.FirstSeenAt = public_key_info.EnrollTime
	result.LastSeenAt = client_info.Ping
	result.LastIp = client_info.IpAddress

	return result, nil
}

// A fast way of getting some client information. Reduces reads from
// the backend by caching as much data as possible - may not be the
// most up to date information but does contain the latest ping
// data. This is mostly used to display the results from the search
// screen.
func (self *Indexer) FastGetApiClient(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) (*api_proto.ApiClient, error) {

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	client_info, err := client_info_manager.Get(ctx, client_id)
	if err != nil {
		return nil, err
	}

	if client_info == nil {
		return nil, errors.New("Invalid client_info")
	}

	labeler := services.GetLabeler(config_obj)
	if labeler == nil {
		return nil, errors.New("Labeler not ready")
	}

	return &api_proto.ApiClient{
		ClientId: client_id,
		Labels:   labeler.GetClientLabels(ctx, config_obj, client_id),
		AgentInformation: &api_proto.AgentInformation{
			Version:   client_info.ClientVersion,
			BuildTime: client_info.BuildTime,
			BuildUrl:  client_info.BuildUrl,
			Name:      client_info.ClientName,
		},
		OsInfo: &api_proto.Uname{
			System:       client_info.System,
			Hostname:     client_info.Hostname,
			Release:      client_info.Release,
			Machine:      client_info.Architecture,
			Fqdn:         client_info.Fqdn,
			MacAddresses: client_info.MacAddresses,
		},
		FirstSeenAt:                 client_info.FirstSeenAt,
		LastSeenAt:                  client_info.Ping,
		LastIp:                      client_info.IpAddress,
		LastInterrogateFlowId:       client_info.LastInterrogateFlowId,
		LastInterrogateArtifactName: client_info.LastInterrogateArtifactName,
	}, nil
}
