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
	"www.velocidex.com/golang/velociraptor/utils"
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

	resolver := NewClientResolver(ctx, config_obj, self)
	defer resolver.Cancel()

	go func() {
		defer resolver.Close()
		for _, child := range children {
			client_id := child.Base()

			// Filter out the clients that do not belong in this
			// org. The users' MRU is currently global and stored in
			// the root org - it contains all clients the user has
			// visited from all orgs.
			if !utils.CompareOrgIds(
				utils.OrgIdFromClientId(client_id), config_obj.OrgId) {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case resolver.In <- client_id:
			}
		}
	}()

	// Return all the valid records
	for api_client := range resolver.Out {
		// Skip clients that are offline
		if in.Filter == api_proto.SearchClientsRequest_ONLINE &&
			now > api_client.LastSeenAt &&
			now-api_client.LastSeenAt > 1000000*60*15 {
			continue
		}

		result.Items = append(result.Items, api_client)
	}

	// Sort the results in reverse order - most recent first.
	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].FirstSeenAt > result.Items[j].FirstSeenAt
	})

	// Page the result properly
	start := int(in.Offset)
	if start > len(result.Items) {
		result.Items = nil
		return result, nil
	}

	end := int(in.Offset + limit)
	if end <= 0 {
		result.Items = nil
		return result, nil
	}

	if end > len(result.Items) {
		end = len(result.Items)
	}
	result.Items = result.Items[start:end]

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

	if in.NameOnly {
		return self.searchClientIndexNameOnly(ctx, config_obj, in, limit)
	}

	// If asked to sort, we need to retrieve a large number of clients
	// and sort the results. This is much slower.
	if in.Sort != api_proto.SearchClientsRequest_UNSORTED {
		hits, err := self.searchClientIndex(ctx, config_obj,
			&api_proto.SearchClientsRequest{
				Limit:  1000,
				Query:  in.Query,
				Filter: in.Filter,
			}, 1000)
		if err != nil {
			return nil, err
		}

		switch in.Sort {
		case api_proto.SearchClientsRequest_SORT_UP:
			sort.Slice(hits.Items, func(x, y int) bool {
				return hits.Items[x].OsInfo.Hostname <
					hits.Items[y].OsInfo.Hostname
			})
		case api_proto.SearchClientsRequest_SORT_DOWN:
			sort.Slice(hits.Items, func(x, y int) bool {
				return hits.Items[x].OsInfo.Hostname >
					hits.Items[y].OsInfo.Hostname
			})
		}

		limit := in.Limit
		if limit > uint64(len(hits.Items)) {
			limit = uint64(len(hits.Items))
		}
		hits.Items = hits.Items[:limit]
		return hits, nil
	}

	// Microseconds
	now := uint64(time.Now().UnixNano() / 1000)
	seen := make(map[string]bool)
	result := &api_proto.SearchClientsResponse{}
	total_count := 0

	resolver := NewClientResolver(ctx, config_obj, self)
	defer resolver.Cancel()

	// Feed the hits to the resolver. This will look up the records in
	// a worker pool.
	go func() {
		defer resolver.Close()

		scope := vql_subsystem.MakeScope()
		prefix, filter := splitSearchTermIntoPrefixAndFilter(scope, in.Query)
		for hit := range self.SearchIndexWithPrefix(ctx, config_obj, prefix) {
			if hit == nil {
				continue
			}

			if filter != nil && !filter.MatchString(hit.Term) {
				continue
			}
			client_id := hit.Entity

			// Uniquify the client ID
			_, pres := seen[client_id]
			if pres {
				continue
			}
			seen[client_id] = true
			select {
			case <-ctx.Done():
				return
			case resolver.In <- client_id:
			}
		}
	}()

	// Return all the valid records
	for api_client := range resolver.Out {
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
			return result, nil
		}
	}

	return result, nil
}

// Name only searches are used for the suggestion box completions.
func (self *Indexer) searchClientIndexNameOnly(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	if !self.Ready() {
		return nil, errors.New("Indexer not ready")
	}

	result := &api_proto.SearchClientsResponse{}
	total_count := 0
	scope := vql_subsystem.MakeScope()

	seen := make(map[string]bool)

	prefix, filter := splitSearchTermIntoPrefixAndFilter(scope, in.Query)
	for hit := range self.SearchIndexWithPrefix(ctx, config_obj, prefix) {
		if hit == nil {
			continue
		}

		if filter != nil && !filter.MatchString(hit.Term) {
			continue
		}

		// This is the client ID for the matching client.
		total_count++
		if uint64(total_count) < in.Offset {
			continue
		}

		// Uniquify the labels
		_, pres := seen[hit.Term]
		if pres {
			continue
		}
		seen[hit.Term] = true

		result.Names = append(result.Names, hit.Term)
		if uint64(len(result.Names)) > limit {
			return result, nil
		}
	}

	sort.Strings(result.Names)

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
