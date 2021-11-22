package utils

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/google/uuid"
)

var (
	idx uint64
)

func GetId() uint64 {
	return atomic.AddUint64(&idx, 1)
}

func GetGUID() int64 {
	u := uuid.New()
	return int64(binary.BigEndian.Uint64(u[0:8]) >> 2)
}
