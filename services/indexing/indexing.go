package indexing

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/search"
)

func StartIndexingService(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	search.LoadIndex(ctx, wg, config_obj)

	wg.Add(1)
	go func() {
		defer wg.Done()

		// For the context to be cancelled.
		<-ctx.Done()

		// Store a snapshot if needed when we terminate
		file_store_factory := file_store.GetFileStore(config_obj)
		search.SnapshotIndex(config_obj)
		file_store_factory.Close()
	}()

	return nil
}
