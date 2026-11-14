package indexing

import (
	"context"
	"errors"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
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

	labeler := services.GetLabeler(config_obj)
	if labeler == nil {
		return nil, errors.New("Labeler not ready")
	}

	labels := labeler.GetClientLabels(ctx, config_obj, client_id)

	client_info, err := services.GetClientInfoItem(ctx, config_obj, client_id,
		func(client_info *actions_proto.ClientInfo) (interface{}, error) {
			result := &api_proto.ApiClient{
				ClientId: client_id,
				Labels:   labels,
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
				LastHuntTimestamp:           client_info.LastHuntTimestamp,
				LastEventTableVersion:       client_info.LastEventTableVersion,
				InFlightFlows:               make(map[string]int64),
			}

			// make a copy of the map
			for k, v := range client_info.InFlightFlows {
				result.InFlightFlows[k] = v
			}

			return result, nil
		})
	if err != nil {
		return nil, err
	}

	res, ok := client_info.(*api_proto.ApiClient)
	if !ok {
		return nil, utils.InvalidArgError
	}

	return res, nil
}
