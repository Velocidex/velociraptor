package indexing

// Implement client searching

import (
	"context"
	"errors"
	"sort"
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
		"mac:",
		"client:",
		"recent:",
		"ip:",
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
func (self *Indexer) searchRecents(
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

	metadata := make([]api_proto.ApiClient, len(children))

	for i, child := range children {
		// Read the MRU ages
		db.GetSubject(config_obj, child, &metadata[i])
	}

	sort.Slice(metadata, func(i, j int) bool {
		return metadata[i].FirstSeenAt > metadata[j].FirstSeenAt
	})

	for _, md := range metadata {
		client_id := md.ClientId
		api_client, err := self.FastGetApiClient(ctx, config_obj, client_id)
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

func (self *Indexer) SearchClients(
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
	case "label", "host", "all", "mac":
		return self.searchClientIndex(ctx, config_obj, in, limit)

	case "client":
		in.Query = term
		return self.searchClientIndex(ctx, config_obj, in, limit)

	case "recent":
		return self.searchRecents(ctx, config_obj, in, principal, term, limit)

	case "ip":
		return self.searchLastIP(ctx, config_obj, in, term, limit)

	default:
		return self.searchVerbs(ctx, config_obj, in, limit)
	}
}

func (self *Indexer) searchClientIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	if !self.Ready() {
		return nil, errors.New("Indexer not ready")
	}

	// Microseconds
	now := uint64(time.Now().UnixNano() / 1000)

	seen := make(map[string]bool)
	result := &api_proto.SearchClientsResponse{}
	total_count := 0
	options := OPTION_CLIENT_RECORDS
	if in.NameOnly {
		options = OPTION_NAME_ONLY
	}

	scope := vql_subsystem.MakeScope()
	prefix, filter := splitSearchTermIntoPrefixAndFilter(scope, in.Query)
	for hit := range self.SearchIndexWithPrefix(ctx, config_obj, prefix) {
		if hit == nil {
			continue
		}

		if filter != nil && !filter.MatchString(hit.Term) {
			continue
		}

		// This is the client ID for the matching client.
		key := hit.Entity
		if options == OPTION_NAME_ONLY {
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
		case OPTION_CLIENT_RECORDS:
			api_client, err := self.FastGetApiClient(ctx, config_obj, hit.Entity)
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

		case OPTION_NAME_ONLY:
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
func (self *Indexer) searchVerbs(ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	terms := []string{}
	items := []*api_proto.ApiClient{}

	term := strings.ToLower(in.Query)
	for _, verb := range verbs {
		if strings.HasPrefix(verb, term) {
			terms = append(terms, verb)
		}
	}

	// Not a verb maybe a hostname
	if uint64(len(terms)) < in.Limit {
		res, err := self.searchClientIndex(ctx, config_obj,
			&api_proto.SearchClientsRequest{
				NameOnly: in.NameOnly,
				Offset:   in.Offset,
				Query:    "host:" + in.Query,
				Limit:    in.Limit,
				Filter:   in.Filter,
			}, limit)
		if err == nil {
			terms = append(terms, res.Names...)
			items = append(items, res.Items...)
		}
	}

	if uint64(len(terms)) < in.Limit {
		res, err := self.searchClientIndex(ctx, config_obj,
			&api_proto.SearchClientsRequest{
				NameOnly: in.NameOnly,
				Offset:   in.Offset,
				Query:    "label:" + in.Query,
				Filter:   in.Filter,
				Limit:    in.Limit,
			}, limit)
		if err == nil {
			terms = append(terms, res.Names...)
			items = append(items, res.Items...)
		}
	}

	return &api_proto.SearchClientsResponse{
		Names: terms,
		Items: items,
	}, nil
}
