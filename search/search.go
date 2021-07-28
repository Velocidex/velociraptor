package search

// Implement client searching

import (
	"context"
	"errors"
	"strings"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
)

func splitIntoOperatorAndTerms(term string) (string, string) {
	parts := strings.SplitN(term, ":", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

// Get the recent clients viewed by the principal sorted in most
// recently used order.
func searchRecents(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	principal string, term string, limit uint64) (
	*api_proto.SearchClientsResponse, error) {
	path_manager := &paths.UserPathManager{principal}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	now := uint64(time.Now().UnixNano() / 1000)
	result := &api_proto.SearchClientsResponse{}
	for _, client_id := range db.SearchClients(config_obj, path_manager.MRU(),
		"mru", "key", in.Offset, in.Limit, datastore.UNSORTED) {
		api_client, err := GetApiClient(
			ctx, config_obj, client_id, false /* detailed */)
		if err != nil {
			continue
		}

		// Skip clients that are offline
		if in.Filter == api_proto.SearchClientsRequest_ONLINE &&
			now > api_client.LastSeenAt &&
			now-api_client.LastSeenAt > 1000000*60*15 {
			continue
		}

		result.Items = append(result.Items, api_client)

		if uint64(len(result.Items)) >= limit {
			break
		}
	}
	return result, nil
}

func SearchClients(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	principal string) (*api_proto.SearchClientsResponse, error) {

	limit := uint64(50)
	if in.Limit > 0 {
		limit = in.Limit
	}

	operator, term := splitIntoOperatorAndTerms(in.Query)
	switch operator {
	case "", "label":
	case "recent":
		return searchRecents(ctx, config_obj, in, principal, term, limit)
	default:
		return nil, errors.New("Invalid search operator " + operator)
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	query_type := ""
	if in.Type == api_proto.SearchClientsRequest_KEY {
		query_type = "key"
	}

	sort_direction := datastore.UNSORTED
	switch in.Sort {
	case api_proto.SearchClientsRequest_SORT_UP:
		sort_direction = datastore.SORT_UP
	case api_proto.SearchClientsRequest_SORT_DOWN:
		sort_direction = datastore.SORT_DOWN
	}

	// If the output is filtered, we need to retrieve as many
	// clients as possible because we may eliminate them with the
	// filter.
	if in.Filter != api_proto.SearchClientsRequest_UNFILTERED {
		limit = 100000
	}

	// Microseconds
	now := uint64(time.Now().UnixNano() / 1000)

	result := &api_proto.SearchClientsResponse{}
	for _, client_id := range db.SearchClients(
		config_obj, constants.CLIENT_INDEX_URN,
		in.Query, query_type, in.Offset, 0, sort_direction) {
		if in.NameOnly || query_type == "key" {
			result.Names = append(result.Names, client_id)
		} else {
			api_client, err := GetApiClient(
				ctx, config_obj, client_id, false /* detailed */)
			if err != nil {
				continue
			}

			// Skip clients that are offline
			if in.Filter == api_proto.SearchClientsRequest_ONLINE &&
				now > api_client.LastSeenAt &&
				now-api_client.LastSeenAt > 1000000*60*15 {
				continue
			}

			result.Items = append(result.Items, api_client)

			if uint64(len(result.Items)) >= limit {
				break
			}
		}
	}

	return result, nil
}
