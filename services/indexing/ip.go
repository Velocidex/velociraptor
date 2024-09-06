package indexing

import (
	"context"
	"strings"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func (self *Indexer) searchLastIP(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	term string, limit uint64) (*api_proto.SearchClientsResponse, error) {

	// It does not make sense to complete on names.
	if in.NameOnly {
		return &api_proto.SearchClientsResponse{}, nil
	}

	result := &api_proto.SearchClientsResponse{}
	total_count := 0

	// Just enumerate all the clients and check their IP.
	scope := vql_subsystem.MakeScope()
	prefix, filter := splitSearchTermIntoPrefixAndFilter(scope, term)

	search_chan, err := self.SearchClientsChan(ctx, scope, config_obj, "all", "")
	if err != nil {
		return nil, err
	}

	now := uint64(time.Now().UnixNano() / 1000)

	for api_client := range search_chan {
		if !strings.HasPrefix(api_client.LastIp, prefix) {
			continue
		}

		if filter != nil && !filter.MatchString(api_client.LastIp) {
			continue
		}

		// Skip clients that are offline
		if in.Filter == api_proto.SearchClientsRequest_ONLINE &&
			now > api_client.LastSeenAt &&
			now-api_client.LastSeenAt > 1000000*60*15 {
			continue
		}

		total_count++
		if uint64(total_count) < in.Offset {
			continue
		}

		result.Items = append(result.Items, api_client)
		if uint64(len(result.Items)) > limit {
			return result, nil
		}
	}

	return result, nil
}

func (self *Indexer) searchLastIPChan(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	term string) (chan *api_proto.ApiClient, error) {

	output_chan := make(chan *api_proto.ApiClient)

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(output_chan)

		prefix, filter := splitSearchTermIntoPrefixAndFilter(scope, term)

		for client_id := range client_info_manager.ListClients(ctx) {
			api_client, err := self.FastGetApiClient(ctx, config_obj, client_id)
			if err != nil {
				continue
			}

			if !strings.HasPrefix(api_client.LastIp, prefix) {
				continue
			}

			if filter != nil && !filter.MatchString(api_client.LastIp) {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- api_client:
			}
		}
	}()

	return output_chan, nil
}
