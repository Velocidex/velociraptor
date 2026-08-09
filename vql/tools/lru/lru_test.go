package lru

import (
	"fmt"
	"testing"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type cacheItem struct {
	Id int
}

func TestLRU(t *testing.T) {
	var evicted []int
	lru := ttlcache.NewCache()

	// We observe that a ttl must be set for the cache to properly
	// evict the oldest members - otherwise it evicts the newest
	// member instead!
	lru.SetTTL(time.Hour)
	lru.SetCacheSizeLimit(4)
	lru.SetExpirationReasonCallback(
		func(key string,
			reason ttlcache.EvictionReason, value interface{}) error {
			item := value.(*cacheItem)
			evicted = append(evicted, item.Id)
			return nil
		})

	for i := 0; i < 10; i++ {
		lru.Set(fmt.Sprintf("%v", i), &cacheItem{
			Id: i,
		})
	}

	goldie.Assert(t, "TestLRU",
		json.MustMarshalIndent(evicted))
}
