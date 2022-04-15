package services

import (
	"context"
	"errors"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/vfilter"
)

var (
	indexer    Indexer
	indexer_mu sync.Mutex
)

func GetIndexer() (Indexer, error) {
	indexer_mu.Lock()
	defer indexer_mu.Unlock()

	if indexer == nil {
		return nil, errors.New("Indexing service not initialized")
	}

	return indexer, nil
}

func RegisterIndexer(m Indexer) {
	indexer_mu.Lock()
	defer indexer_mu.Unlock()

	indexer = m
}

type Indexer interface {
	// Set a search term on a client
	SetIndex(client_id, term string) error

	// Clear a search term on a client
	UnsetIndex(client_id, term string) error

	// Search the index for clients matching the term
	SearchIndexWithPrefix(
		ctx context.Context,
		config_obj *config_proto.Config,
		prefix string) <-chan *api_proto.IndexRecord

	SearchClientsChan(
		ctx context.Context,
		scope vfilter.Scope,
		config_obj *config_proto.Config,
		search_term string, principal string) (chan *api_proto.ApiClient, error)

	SearchClients(
		ctx context.Context,
		config_obj *config_proto.Config,
		in *api_proto.SearchClientsRequest,
		principal string) (*api_proto.SearchClientsResponse, error)

	SetSimpleIndex(
		config_obj *config_proto.Config,
		index_urn api.DSPathSpec,
		entity string,
		keywords []string) error

	UnsetSimpleIndex(
		config_obj *config_proto.Config,
		index_urn api.DSPathSpec,
		entity string,
		keywords []string) error

	CheckSimpleIndex(
		config_obj *config_proto.Config,
		index_urn api.DSPathSpec,
		entity string,
		keywords []string) error

	UpdateMRU(
		config_obj *config_proto.Config,
		user_name string, client_id string) error

	FastGetApiClient(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string) (*api_proto.ApiClient, error)
}
