package process

import (
	"context"

	"github.com/Velocidex/disklru"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	invalidProcessEntryError = utils.Wrap(utils.InvalidArgError,
		"Invalid Process Entry Error")
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

	HouseKeepOnce()
}

type MemoryLRU struct {
	*cache.LRUCache
	opts Options
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
	cache := cache.NewLRUCache(int64(opts.MaxSize))
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

func NewDiskCache(
	ctx context.Context, opts Options) (*DiskLRU, error) {
	res, err := disklru.NewDiskLRU(ctx,
		disklru.Options(opts.Options))
	if err != nil {
		return nil, err
	}

	res.SetEncoder(ProcessEntryEncoder{})

	return &DiskLRU{
		DiskLRU: res,
		opts:    opts,
	}, nil
}

type ProcessEntryEncoder struct{}

func (self ProcessEntryEncoder) Encode(in interface{}) ([]byte, error) {
	entry, ok := in.(*ProcessEntry)
	if !ok {
		return nil, invalidProcessEntryError
	}

	// Encode a link
	if entry.RealId != "" {
		return []byte(json.Format(`{"Id":%q,"RealId":%q}`,
			entry.Id, entry.RealId)), nil
	}

	// Encode a full entry
	return []byte(json.Format(
		`{"Id":%q,"ParentId":%q,"StartTime":%q,"LastSyncTime":%q,"EndTime":%q,"JSONData":%q,"Children":%q}`,
		entry.Id, entry.ParentId,
		entry.StartTime, entry.LastSyncTime, entry.EndTime,
		entry.JSONData, entry.Children)), nil
}

func (self ProcessEntryEncoder) Decode(in []byte) (interface{}, error) {
	res := &ProcessEntry{}
	err := json.Unmarshal(in, &res)

	return res, err
}

type LRUClock int

func (self LRUClock) Now() int64 {
	return utils.GetTime().Now().UnixNano()
}
