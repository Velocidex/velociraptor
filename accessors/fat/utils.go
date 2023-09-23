package fat

import (
	fat "github.com/Velocidex/go-fat/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	"www.velocidex.com/golang/vfilter"
)

func GetFatContext(scope vfilter.Scope,
	device, fullpath *accessors.OSPath, accessor string) (
	result *fat.FATContext, err error) {
	if device == nil {
		device, err = fullpath.Delegate(scope)
		if err != nil {
			return nil, err
		}
		accessor = fullpath.DelegateAccessor()
	}

	lru_size := vql_subsystem.GetIntFromRow(
		scope, scope, constants.NTFS_CACHE_SIZE)

	paged_reader, err := readers.NewPagedReader(
		scope, accessor, device, int(lru_size))

	return fat.GetFATContext(paged_reader)
}
