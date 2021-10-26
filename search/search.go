package search

// Implement client searching

import (
	"context"
	"strings"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	verbs = []string{
		"label:",
		"host:",
		"client:",
		"recent:",
	}
)

func splitIntoOperatorAndTerms(term string) (string, string) {
	if term == "all" {
		return "all", ""
	}

	// Client IDs can be searched directly.
	if strings.HasPrefix(term, "C.") || strings.HasPrefix(term, "c.") {
		return "client", term
	}

	parts := strings.SplitN(term, ":", 2)
	if len(parts) == 1 {
		// Bare search terms mean hostname or fqdn
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

	children, err := db.ListChildren(config_obj, path_manager.MRUIndex())
	if err != nil {
		return nil, err
	}

	// Sort the children in reverse order - most recent first.
	total_count := 0
	for i := len(children) - 1; i >= 0; i-- {
		client_id := children[i].Base()
		api_client, err := GetApiClient(
			ctx, config_obj, client_id, false /* detailed */)
		if err != nil {
			continue
		}

		total_count++
		if uint64(total_count) < in.Offset {
			continue
		}

		// Skip clients that are offline
		if in.Filter == api_proto.SearchClientsRequest_ONLINE &&
			now > api_client.LastSeenAt &&
			now-api_client.LastSeenAt > 1000000*60*15 {
			continue
		}

		result.Items = append(result.Items, api_client)

		if uint64(len(result.Items)) > limit {
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
	case "label", "host", "client", "all":
		return searchClientIndex(ctx, config_obj, in, limit)

	case "recent":
		return searchRecents(ctx, config_obj, in, principal, term, limit)

	default:
		return searchVerbs(ctx, config_obj, in, limit)
	}
}

func searchClientIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	// Microseconds
	now := uint64(time.Now().UnixNano() / 1000)

	seen := make(map[string]bool)
	result := &api_proto.SearchClientsResponse{}
	total_count := 0
	options := OPTION_ENTITY
	if in.Type == api_proto.SearchClientsRequest_KEY {
		options = OPTION_KEY
	}

	scope := vql_subsystem.MakeScope()
	prefix, filter := splitSearchTermIntoPrefixAndFilter(scope, in.Query)
	if filter != nil {
		options = OPTION_KEY
	}

	for hit := range SearchIndexWithPrefix(
		ctx, config_obj, prefix, options) {
		if hit == nil {
			continue
		}

		if filter != nil && !filter.MatchString(hit.Term) {
			continue
		}

		key := hit.Entity
		if options == OPTION_KEY {
			key = hit.Term
		}

		// Uniquify the client ID
		_, pres := seen[key]
		if pres {
			continue
		}
		seen[key] = true

		total_count++
		if uint64(total_count) < in.Offset {
			continue
		}

		switch options {
		case OPTION_ENTITY:
			api_client, err := FastGetApiClient(ctx, config_obj, hit.Entity)
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
			if uint64(len(result.Items)) > limit {
				return result, nil
			}

		case OPTION_KEY:
			result.Names = append(result.Names, hit.Term)
			if uint64(len(result.Names)) > limit {
				return result, nil
			}
		}

	}

	return result, nil
}

// Free form search term, try to fill in as many suggestions as
// possible.
func searchVerbs(ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	terms := []string{}
	term := strings.ToLower(in.Query)
	for _, verb := range verbs {
		if strings.HasPrefix(verb, term) {
			terms = append(terms, verb)
		}
	}

	// Not a verb maybe a hostname
	if uint64(len(terms)) < in.Limit {
		res, _ := searchClientIndex(ctx, config_obj,
			&api_proto.SearchClientsRequest{
				Type:   in.Type,
				Offset: in.Offset,
				Query:  "host:" + in.Query,
				Limit:  in.Limit,
			}, limit)
		terms = append(terms, res.Names...)
	}

	if uint64(len(terms)) < in.Limit {
		res, _ := searchClientIndex(ctx, config_obj,
			&api_proto.SearchClientsRequest{
				Type:   in.Type,
				Offset: in.Offset,
				Query:  "label:" + in.Query,
				Limit:  in.Limit,
			}, limit)
		terms = append(terms, res.Names...)
	}

	return &api_proto.SearchClientsResponse{
		Names: terms,
	}, nil
}
