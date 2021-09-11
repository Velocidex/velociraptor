package services

import (
	"context"
	"errors"
	"sync"

	"github.com/Velocidex/ordereddict"
)

// The broadcast service allows VQL to implement fan out
// behavior. This is useful for multiple queries that need to operate
// on the result of a single query efficiently.
//
// An event generator is a query which produces rows and has a
// name. Users can use the source() plugin to receive events from the
// query or pull events directly from the generator.
//
// LET Generator <= generate(name="MyGenerator", query={
//    SELECT * FROM parse_mft(...)
//  }, delay=2)
//
//
//  SELECT * FROM chain(
//   a={ SELECT * FROM source(name="MyGenerator") WHERE X = 1 },
//   b={ SELECT * FROM Generator WHERE X = 2 },
//   c={ SELECT * FROM Generator WHERE X = 3 },
//   async=TRUE)
//
// In the above:

// 1. A Generator object is created with the name MyGenerator. The
//    generator will wait 2 seconds before starting the query to
//    produce rows.

// 2. In the next query, the chain() plugin starts several queries,
//    all drawing events from the same generator. Each event will be
//    duplicated to all members and the results will be combined. Due
//    to the async=TRUE option, the queries will all run in parallel.

var (
	broadcast_mu     sync.Mutex
	broadcastService BroadcastService

	AlreadyRegisteredError = errors.New("Already Registered")
)

func RegisterBroadcast(b BroadcastService) {
	broadcast_mu.Lock()
	defer broadcast_mu.Unlock()

	broadcastService = b
}

func GetBroadcastService() (BroadcastService, error) {
	broadcast_mu.Lock()
	defer broadcast_mu.Unlock()

	if broadcastService == nil {
		return nil, errors.New("Broadcast service not ready")
	}

	return broadcastService, nil
}

type BroadcastService interface {
	RegisterGenerator(input <-chan *ordereddict.Dict, name string) error
	Watch(ctx context.Context, name string) (
		output <-chan *ordereddict.Dict, cancel func(), err error)
}
