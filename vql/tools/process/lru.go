package process

import (
	"context"

	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/tools/lru"
)

var (
	invalidProcessEntryError = utils.Wrap(utils.InvalidArgError,
		"Invalid Process Entry Error")
)

type Stats lru.Stats
type LRUCache lru.LRUCache
type Options lru.Options

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

func NewLRUCache(ctx context.Context, opts lru.Options) (lru.LRUCache, error) {
	if opts.Filename == "" {
		return lru.NewMemoryCache(opts), nil

	}
	res, err := lru.NewDiskCache(ctx, opts)
	if err != nil {
		return nil, err
	}

	res.SetEncoder(ProcessEntryEncoder{})

	return res, nil
}
