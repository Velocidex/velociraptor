package utils

import "sync/atomic"

var (
	idx uint64
)

func GetId() uint64 {
	return atomic.AddUint64(&idx, 1)
}
