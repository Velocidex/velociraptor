package uploads

import (
	"sort"
)

// Find the current range blocking offset and the next range after
// offset. If the offset does not correspond to a current range, then
// current_range is nil. Similarly if there is no next range,
// next_range is nil.

// We assume ranges are sorted of Offset and do not overlap.
func GetNextRange(offset int64, ranges []*Range) (current_range, next_range *Range) {
	idx := sort.Search(len(ranges), func(i int) bool {
		end := ranges[i].Offset + ranges[i].Length
		return end > offset
	})

	if idx >= len(ranges) {
		return nil, nil
	}

	if ranges[idx].Offset <= offset {
		current_range = ranges[idx]
	} else {
		next_range = ranges[idx]
	}

	return current_range, next_range
}
