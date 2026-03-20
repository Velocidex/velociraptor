package lru

import (
	"context"

	"github.com/Velocidex/disklru"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
)

type Options struct {
	disklru.Options
	MaxChildren int
}

func (self Options) MarshalJSON() ([]byte, error) {
	return []byte(json.Format(
		`{"Filename":%q,"MaxChildren":%q,"MaxSize":%q,"MaxExpirySec":%q}`,
		self.Filename, self.MaxChildren,
		self.MaxSize, self.MaxExpirySec)), nil
}

type Stats struct {
	Length, Size, Capacity, Evictions int64
	Hits, Misses                      int64
	Opts                              Options
}

type CacheItem struct {
	Key   string
	Value interface{}
}

type LRUCache interface {
	Get(key string) (interface{}, bool)
	Peek(key string) (interface{}, bool)
	Set(key string, value interface{})
	Items() []CacheItem
	Delete(key string) bool
	Stats() Stats
	Purge()
	Close()

	HouseKeepOnce()
}

type MemoryLRU struct {
	*cache.LRUCache
	opts Options
}

func (self *MemoryLRU) Purge() {
	self.LRUCache.Clear()
}

func (self *MemoryLRU) Close() {
	self.LRUCache.Clear()
}

func (self *MemoryLRU) HouseKeepOnce() {}

func (self *MemoryLRU) Get(key string) (interface{}, bool) {
	res, pres := self.LRUCache.Get(key)
	if !pres {
		return nil, false
	}

	return res, pres
}

func (self *MemoryLRU) Peek(key string) (interface{}, bool) {
	res, pres := self.LRUCache.Peek(key)
	if !pres {
		return nil, false
	}

	return res, pres
}

func (self *MemoryLRU) Set(key string, value interface{}) {
	self.LRUCache.Set(key, value)
}

func (self *MemoryLRU) Stats() (res Stats) {
	s := self.LRUCache.Stats()
	return Stats{
		Length:    s.Length,
		Size:      s.Size,
		Capacity:  s.Capacity,
		Evictions: s.Evictions,
		Hits:      s.Hits,
		Misses:    s.Misses,
		Opts:      self.opts,
	}
}

func (self *MemoryLRU) Items() (res []CacheItem) {
	for _, i := range self.LRUCache.Items() {
		res = append(res, CacheItem{Key: i.Key, Value: i.Value})
	}
	return res
}

func NewMemoryCache(opts Options) *MemoryLRU {
	max_size := int64(opts.MaxSize)
	cache := cache.NewLRUCache(max_size)
	return &MemoryLRU{
		LRUCache: cache,
		opts:     opts,
	}
}

type DiskLRU struct {
	*disklru.DiskLRU
	opts Options
}

func (self *DiskLRU) Set(key string, value interface{}) {
	self.DiskLRU.Set(key, value)
}

func (self *DiskLRU) Get(key string) (interface{}, bool) {
	res, err := self.DiskLRU.Get(key)
	if err != nil {
		return nil, false
	}

	return res, true
}

func (self *DiskLRU) Peek(key string) (interface{}, bool) {
	res, err := self.DiskLRU.Peek(key)
	if err != nil {
		return nil, false
	}

	return res, true
}

func (self *DiskLRU) Stats() (res Stats) {
	s := self.DiskLRU.Stats()
	return Stats{
		Length:    s.Length,
		Size:      s.Size,
		Capacity:  s.Capacity,
		Evictions: s.Evictions,
		Hits:      s.Hits,
		Misses:    s.Misses,
		Opts:      self.opts,
	}
}

func (self *DiskLRU) Items() (res []CacheItem) {
	for _, i := range self.DiskLRU.Items() {
		res = append(res, CacheItem{Key: i.Key, Value: i.Value})
	}
	return res
}

func (self *DiskLRU) Purge() {
	self.DiskLRU.Purge()
}

func (self *DiskLRU) Close() {
	self.DiskLRU.Close()
}

func NewDiskCache(
	ctx context.Context, opts Options) (*DiskLRU, error) {
	res, err := disklru.NewDiskLRU(ctx,
		disklru.Options(opts.Options))
	if err != nil {
		return nil, err
	}

	return &DiskLRU{
		DiskLRU: res,
		opts:    opts,
	}, nil
}

type LRUClock int

func (self LRUClock) Now() int64 {
	return utils.GetTime().Now().UnixNano()
}
