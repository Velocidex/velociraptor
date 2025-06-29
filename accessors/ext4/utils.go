package ext4

import (
	ext4 "github.com/Velocidex/go-ext4/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	"www.velocidex.com/golang/vfilter"
)

func GetExt4Context(scope vfilter.Scope,
	device, fullpath *accessors.OSPath, accessor string) (
	result *ext4.EXT4Context, err error) {

	if device == nil {
		device, err = fullpath.Delegate(scope)
		if err != nil {
			return nil, err
		}
		accessor = fullpath.DelegateAccessor()
	}

	return GetExt4Cache(scope, device, accessor)
}

func GetExt4Cache(scope vfilter.Scope,
	device *accessors.OSPath, accessor string) (*ext4.EXT4Context, error) {
	key := "ext4_cache" + device.String() + accessor

	// Get the cache context from the root scope's cache
	cache_ctx, ok := vql_subsystem.CacheGet(scope, key).(*ext4.EXT4Context)
	if !ok {
		lru_size := vql_subsystem.GetIntFromRow(
			scope, scope, constants.NTFS_CACHE_SIZE)

		paged_reader, err := readers.NewAccessorReader(
			scope, accessor, device, int(lru_size))
		if err != nil {
			return nil, err
		}

		cache_ctx, err = ext4.GetEXT4Context(paged_reader)
		if err != nil {
			return nil, err
		}
		vql_subsystem.CacheSet(scope, key, cache_ctx)

		// Close the device when we are done with this query.
		err = vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			paged_reader.Close()
		})
		if err != nil {
			return nil, err
		}
	}

	return cache_ctx, nil
}
