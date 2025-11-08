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
	mu       sync.Mutex
	response *UploadResponse
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

	var once sync.Once

	self.mu.Lock()
	return self.response, func(response *UploadResponse) {
		once.Do(func() {
			if response != nil {
				self.response = response
			}
			self.mu.Unlock()
		})
	}
}

// Manage the uploader cache - this is used to deduplicate files that
// are uploaded multiple time so they only upload one file.
func DeduplicateUploads(
	accessor string,
	scope vfilter.Scope,
	store_as_name *accessors.OSPath) (
	*UploadResponse, func(response *UploadResponse)) {

	cached_response := getCacheResponse(accessor, scope, store_as_name)
	return cached_response.LeaseResponse()
}

func getCacheResponse(
	accessor string, scope vfilter.Scope,
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
	key := accessors.GetCanonicalFilename(accessor, scope, store_as_name)
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
