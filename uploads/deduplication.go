package uploads

import (
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	dedup_mu sync.Mutex
)

type cachedUploadResponse struct {
	mu sync.Mutex

	response *UploadResponse
}

// Lease the response for a time, when the caller is done with it,
// they can return it by calling the callback.
func (self *cachedUploadResponse) LeaseResponse() (
	response *UploadResponse, closer func(response *UploadResponse)) {

	self.mu.Lock()
	if self.response == nil {
		return nil, func(response *UploadResponse) {
			if response != nil {
				self.response = response
			}
			self.mu.Unlock()
		}
	}

	return self.response, func(response *UploadResponse) {
		self.mu.Unlock()
	}
}

// Manage the uploader cache - this is used to deduplicate files that
// are uploaded multiple time so they only upload one file.
func DeduplicateUploads(scope vfilter.Scope,
	store_as_name *accessors.OSPath) (
	*UploadResponse, func(response *UploadResponse)) {

	cached_response := getCacheResponse(scope, store_as_name)
	return cached_response.LeaseResponse()
}

func getCacheResponse(scope vfilter.Scope,
	store_as_name *accessors.OSPath) *cachedUploadResponse {

	dedup_mu.Lock()
	defer dedup_mu.Unlock()

	root_scope := vql_subsystem.GetRootScope(scope)
	cache_any := vql_subsystem.CacheGet(root_scope, UPLOAD_CTX)
	if utils.IsNil(cache_any) {
		cache_any = ordereddict.NewDict()
	}

	cache, ok := cache_any.(*ordereddict.Dict)
	if !ok {
		cache = ordereddict.NewDict()
	}
	defer vql_subsystem.CacheSet(root_scope, UPLOAD_CTX, cache)

	// Search for the cached upload response.
	key := store_as_name.String()
	cached_response_any, pres := cache.Get(key)
	if pres {
		cached_response, ok := cached_response_any.(*cachedUploadResponse)
		if ok {
			return cached_response
		}
	}

	// If there is no cached item, we need to add a placeholder, so
	// other uploaders will wait for us to complete.
	placeholder := &cachedUploadResponse{}
	cache.Set(key, placeholder)

	return placeholder
}

// Add the result into the cache
func CacheUploadResult(scope vfilter.Scope,
	store_as_name *accessors.OSPath,
	response *UploadResponse) {

	if response == nil {
		return
	}

	dedup_mu.Lock()
	defer dedup_mu.Unlock()

	root_scope := vql_subsystem.GetRootScope(scope)
	cache_any := vql_subsystem.CacheGet(root_scope, UPLOAD_CTX)
	if utils.IsNil(cache_any) {
		return
	}

	cache, ok := cache_any.(*ordereddict.Dict)
	if ok {
		key := store_as_name.String()
		cached_response_any, pres := cache.Get(key)
		if !pres {
			cached_response_any = &cachedUploadResponse{
				response: response,
			}
		}

		cached_response, ok := cached_response_any.(*cachedUploadResponse)
		if ok {
			cached_response.response = response
		} else {
			// Should not really happen but if it does, we just cache
			// it anyway
			cached_response = &cachedUploadResponse{
				response: response,
			}
		}
		cache.Set(key, cached_response)
	}
}
