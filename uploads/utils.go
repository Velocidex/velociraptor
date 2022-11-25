package uploads

import (
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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
