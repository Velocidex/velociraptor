package uploads

import actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"

func ShouldPadFile(index *actions_proto.Index) bool {
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

	// The total size is not too large - expand it.
	if total_size < 100*1024*1024 {
		return true
	}

	// The total size is within a small factor of the actual data
	// size, we should expand it.
	if total_size/data_size <= 2 {
		return true
	}

	return false
}
