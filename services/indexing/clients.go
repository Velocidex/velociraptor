package indexing

import (
	"context"
	"errors"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

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

	return self._FastGetApiClient(ctx, config_obj, client_id, client_info_manager)

}

func (self *Indexer) _FastGetApiClient(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	client_info_manager services.ClientInfoManager) (*api_proto.ApiClient, error) {

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
		InFlightFlows:               client_info.InFlightFlows,
	}, nil
}
