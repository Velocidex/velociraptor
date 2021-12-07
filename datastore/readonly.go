// Read only memory datastore implementation - read from directory but
// writes stay in memory.

package datastore

import "context"

var (
	read_only_imp *ReadOnlyDataStore
)

type ReadOnlyDataStore struct {
	*MemcacheFileDataStore
}

func NewReadOnlyDataStore() *ReadOnlyDataStore {
	result := &ReadOnlyDataStore{&MemcacheFileDataStore{
		cache:  NewMemcacheDataStore(),
		writer: make(chan *Mutation),
		ctx:    context.Background(),
	}}

	go func() {
		for range result.writer {
		}
	}()

	return result
}
