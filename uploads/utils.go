package uploads

import (
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func ShouldPadFile(
	config_obj *config_proto.Config,
	index *actions_proto.Index) bool {
	if index == nil || len(index.Ranges) == 0 {
		return false
	}

	// Figure out how sparse the file is - some sparse files are
	// incredibly large and we should not expand them. Specifically
	// the $J file is is incredibly sparse and can be very large.
	var data_size, total_size int64

	for _, i := range index.Ranges {
		data_size += i.FileLength
		total_size += i.Length
	}

	// Default 100mb
	max_sparse_expand_size := uint64(100 * 1024 * 1024)
	if config_obj.Defaults != nil &&
		config_obj.Defaults.MaxSparseExpandSize > 0 {
		max_sparse_expand_size = config_obj.Defaults.MaxSparseExpandSize
	}

	// The total size is not too large - expand it.
	if uint64(total_size) < max_sparse_expand_size {
		return true
	}

	// The total size is within a small factor of the actual data
	// size, we should expand it.
	if total_size/data_size <= 2 {
		return true
	}

	return false
}

// Manage the uploader cache - this is used to deduplicate files that
// are uploaded multiple time so they only upload one file.
func DeduplicateUploads(scope vfilter.Scope,
	store_as_name *accessors.OSPath) (*UploadResponse, bool) {
	cache_any := vql_subsystem.CacheGet(scope, UPLOAD_CTX)
	if utils.IsNil(cache_any) {
		cache_any = ordereddict.NewDict()
		vql_subsystem.CacheSet(scope, UPLOAD_CTX, cache_any)
	}

	cache, ok := cache_any.(*ordereddict.Dict)
	if ok {
		key := store_as_name.String()
		result_any, pres := cache.Get(key)
		if pres {
			result, ok := result_any.(*UploadResponse)
			if ok {
				return result, true
			}
		}
	}
	return nil, false
}

func CacheUploadResult(scope vfilter.Scope,
	store_as_name *accessors.OSPath,
	result *UploadResponse) {
	cache_any := vql_subsystem.CacheGet(scope, UPLOAD_CTX)
	if utils.IsNil(cache_any) {
		cache_any = ordereddict.NewDict()
		vql_subsystem.CacheSet(scope, UPLOAD_CTX, cache_any)
	}

	cache, ok := cache_any.(*ordereddict.Dict)
	if ok {
		key := store_as_name.String()
		cache.Set(key, result)
	}
}
