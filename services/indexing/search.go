package indexing

// Implement client searching

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	// List of built in verbs.
	buiilt_in_verbs = []string{
		"label:",
		"host:",
		"mac:",
		"client:",
		"recent:",
		"ip:",
	}
)

func (self *Indexer) getVerbs() (res []string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if len(self._verbs) == 0 {
		self._verbs = append(self._verbs, buiilt_in_verbs...)
		if self.config_obj.Defaults != nil {
			for _, operator := range self.config_obj.Defaults.IndexedClientMetadata {
				self._verbs = append(self._verbs, operator+":")
			}
		}
	}

	return self._verbs
}

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
	path_manager := &paths.UserPathManager{Name: principal}
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

	result.Total = uint64(len(result.Items))
	result.SearchTerm = in

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

	var custom_verbs []string
	if config_obj.Defaults != nil {
		custom_verbs = config_obj.Defaults.IndexedClientMetadata
	}

	operator, term := splitIntoOperatorAndTerms(in.Query)
	switch operator {
	case "label":
		if term == "none" {
			return self.searchUnlabeledClients(ctx, config_obj, in, limit)
		}
		return self.searchClientIndex(ctx, config_obj, in, limit)

	case "host", "all", "mac":
		return self.searchClientIndex(ctx, config_obj, in, limit)

	case "client":
		in.Query = term
		return self.searchClientIndex(ctx, config_obj, in, limit)

	case "recent":
		return self.searchRecents(ctx, config_obj, in, principal, term, limit)

	case "ip":
		return self.searchLastIP(ctx, config_obj, in, term, limit)

	default:
		if utils.InString(custom_verbs, operator) {
			return self.searchClientIndex(ctx, config_obj, in, limit)
		}

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
				Limit:  100000,
				Query:  in.Query,
				Filter: in.Filter,
			}, 100000)
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

		if in.Offset > uint64(len(hits.Items)) {
			hits.Items = nil
			return hits, nil
		}

		if in.Offset > 0 {
			hits.Items = hits.Items[in.Offset:]
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
	total_count := uint64(0)

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
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
		client_id := hit.Entity

		// Uniquify the client ID
		_, pres := seen[client_id]
		if pres {
			continue
		}
		seen[client_id] = true

		// Skip clients that are offline
		if in.Filter == api_proto.SearchClientsRequest_ONLINE {
			stats, err := client_info_manager.GetStats(ctx, client_id)
			if err != nil {
				continue
			}

			// SKip clients that are too old
			if now > stats.Ping &&
				now-stats.Ping > 1000000*60*15 {
				continue
			}
		}

		total_count++
		if uint64(total_count) < in.Offset {
			continue
		}

		if uint64(len(result.Items)) <= limit {
			api_client, err := self._FastGetApiClient(ctx, self.config_obj,
				client_id, client_info_manager)
			if err != nil {
				total_count--
				continue
			}

			result.Items = append(result.Items, api_client)
		}
	}

	result.Total = total_count
	result.SearchTerm = in
	return result, nil
}

// Return only clients that are unlabeled.
func (self *Indexer) searchUnlabeledClients(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.SearchClientsRequest,
	limit uint64) (*api_proto.SearchClientsResponse, error) {

	// If asked to sort, we need to retrieve a large number of clients
	// and sort the results. This is much slower.
	if in.Sort != api_proto.SearchClientsRequest_UNSORTED {
		hits, err := self.searchUnlabeledClients(ctx, config_obj,
			&api_proto.SearchClientsRequest{
				Limit:  100000,
				Query:  in.Query,
				Filter: in.Filter,
			}, 100000)
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

		if in.Offset > uint64(len(hits.Items)) {
			hits.Items = nil
			return hits, nil
		}

		if in.Offset > 0 {
			hits.Items = hits.Items[in.Offset:]
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
	result := &api_proto.SearchClientsResponse{}
	total_count := uint64(0)

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	labeler := services.GetLabeler(config_obj)
	for client_id := range client_info_manager.ListClients(ctx) {
		if len(labeler.GetClientLabels(ctx, config_obj, client_id)) > 0 {
			continue
		}

		// Skip clients that are offline
		if in.Filter == api_proto.SearchClientsRequest_ONLINE {
			stats, err := client_info_manager.GetStats(ctx, client_id)
			if err != nil {
				continue
			}

			// SKip clients that are too old
			if now > stats.Ping &&
				now-stats.Ping > 1000000*60*15 {
				continue
			}
		}

		total_count++
		if uint64(total_count) < in.Offset {
			continue
		}

		if uint64(len(result.Items)) <= limit {
			api_client, err := self._FastGetApiClient(ctx, self.config_obj,
				client_id, client_info_manager)
			if err != nil {
				total_count--
				continue
			}

			result.Items = append(result.Items, api_client)
		}
	}

	result.Total = total_count
	result.SearchTerm = in
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

	total := uint64(0)
	terms := []string{}

	// Dedup by client id
	items := ordereddict.NewDict()

	term := strings.ToLower(in.Query)
	for _, verb := range self.getVerbs() {
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
			for _, i := range res.Items {
				items.Update(i.ClientId, i)
			}
			total += res.Total
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
			for _, i := range res.Items {
				items.Update(i.ClientId, i)
			}
			total += res.Total
		}
	}

	res := &api_proto.SearchClientsResponse{
		Names:      terms,
		Total:      total,
		SearchTerm: in,
	}

	for _, v := range items.Values() {
		res.Items = append(res.Items, v.(*api_proto.ApiClient))
	}

	return res, nil
}
