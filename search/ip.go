package search

import (
	"context"
	"strings"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func searchLastIP(
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

	search_chan, err := SearchClientsChan(ctx, scope, config_obj, "all", "")
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
