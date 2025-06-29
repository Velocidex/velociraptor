package indexing

// Implement client searching with channel based API

import (
	"context"
	"errors"
	"regexp"
	"strings"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// Get the recent clients viewed by the principal sorted in most
// recently used order.
func (self *Indexer) searchRecentsChan(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	principal string) (
	chan *api_proto.ApiClient, error) {

	path_manager := &paths.UserPathManager{Name: principal}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	children, err := db.ListChildren(config_obj, path_manager.MRUIndex())
	if err != nil {
		return nil, err
	}

	output_chan := make(chan *api_proto.ApiClient)

	go func() {
		defer close(output_chan)

		// Sort the children in reverse order - most recent first.
		for i := len(children) - 1; i >= 0; i-- {
			client_id := children[i].Base()
			api_client, err := self.FastGetApiClient(
				ctx, config_obj, client_id)
			if err != nil {
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

func (self *Indexer) SearchClientsChan(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	search_term string, principal string) (chan *api_proto.ApiClient, error) {

	var custom_verbs []string
	if config_obj.Defaults != nil {
		custom_verbs = config_obj.Defaults.IndexedClientMetadata
	}

	operator, term := splitIntoOperatorAndTerms(search_term)
	switch operator {
	case "label":
		if term == "none" {
			return self.searchUnlabeledClientsChan(ctx, config_obj)
		}
		// Include the operator in these search terms
		return self.searchClientIndexChan(ctx, scope, config_obj, search_term)

	case "host", "all", "mac":
		return self.searchClientIndexChan(ctx, scope, config_obj, search_term)

	case "client":
		return self.searchClientIndexChan(ctx, scope, config_obj, term)

	case "ip":
		return self.searchLastIPChan(ctx, scope, config_obj, term)

	case "":
		return self.searchClientIndexChan(ctx, scope, config_obj, "host:"+term)

	case "recent":
		return self.searchRecentsChan(ctx, scope, config_obj, principal)

	default:
		if utils.InString(custom_verbs, operator) {
			return self.searchClientIndexChan(ctx, scope, config_obj, search_term)
		}

		return nil, errors.New("Invalid search operator " + operator)
	}
}

func (self *Indexer) searchClientIndexChan(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	search_term string) (chan *api_proto.ApiClient, error) {

	// The search term may contain wild cards but in the index we can
	// only search for prefixes. So we need to first extract the
	// search prefix then apply the regex to filter out the hits based
	// on the full search term.
	prefix, filter := splitSearchTermIntoPrefixAndFilter(scope, search_term)

	output_chan := make(chan *api_proto.ApiClient)

	go func() {
		defer close(output_chan)

		// Microseconds
		seen := make(map[string]bool)
		for hit := range self.SearchIndexWithPrefix(ctx, config_obj, prefix) {
			if hit == nil {
				continue
			}

			// If the search term is complicated we need to check the
			// filter against the retrieved term.
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

			api_client, err := self.FastGetApiClient(ctx, config_obj, client_id)
			if err != nil {
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

func (self *Indexer) searchUnlabeledClientsChan(
	ctx context.Context,
	config_obj *config_proto.Config) (chan *api_proto.ApiClient, error) {

	output_chan := make(chan *api_proto.ApiClient)

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	labeler := services.GetLabeler(config_obj)

	go func() {
		defer close(output_chan)

		for client_id := range client_info_manager.ListClients(ctx) {
			if len(labeler.GetClientLabels(ctx, config_obj, client_id)) > 0 {
				continue
			}

			api_client, err := self.FastGetApiClient(ctx, config_obj, client_id)
			if err != nil {
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

// When searching the index, the user may provide wild cards.
func splitSearchTermIntoPrefixAndFilter(
	scope vfilter.Scope, search_term string) (string, *regexp.Regexp) {

	parts := strings.Split(search_term, "*")
	// No wild cards present
	if len(parts) == 1 {
		return search_term, nil
	}

	// Last component is a wildcard, just ignore it (e.g. win* )
	if len(parts) == 2 && parts[1] == "" {
		return parts[0], nil
	}

	// Try to interpret the filter as a glob
	filter_regex := "(?i)" + glob.FNmatchTranslate(search_term)
	filter, err := regexp.Compile(filter_regex)
	if err != nil {
		scope.Log("ClientSearch while Matching %v: %v", search_term, err)
		return parts[0], nil
	}

	return parts[0], filter
}
