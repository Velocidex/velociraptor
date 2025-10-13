package uploads

import (
	"sync"
	"sync/atomic"

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
	mu        sync.Mutex
	is_locked int64

	response *UploadResponse
}

func (self *cachedUploadResponse) isLocked() bool {
	return atomic.LoadInt64(&self.is_locked) != 0
}

// Lease the response for a time, when the caller is done with it,
// they can return it by calling the closer callback. It is safe to
// call the closer multiple times - only the first call will take an
// effect.
//
// result, closer := Deduplicate(....)
// defer closer(result) // Ensure that the closer is called for all error paths.
// ...
// Calculate a better result
// closer(result) // This will set the result in the cache.
// The defer call will have no effect if a better result was obtained.
func (self *cachedUploadResponse) LeaseResponse() (
	response *UploadResponse, closer func(response *UploadResponse)) {

	self.mu.Lock()
	atomic.StoreInt64(&self.is_locked, 1)

	if self.response == nil {
		return nil, func(response *UploadResponse) {
			if !self.isLocked() {
				return
			}

			if response != nil {
				self.response = response
			}
			self.mu.Unlock()
			atomic.StoreInt64(&self.is_locked, 0)
		}
	}

	return self.response, func(response *UploadResponse) {
		self.mu.Unlock()
		atomic.StoreInt64(&self.is_locked, 0)
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
