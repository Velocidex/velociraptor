package indexing

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/search"
)

func StartIndexingService(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	search.LoadIndex(ctx, wg, config_obj)

	return nil
}
