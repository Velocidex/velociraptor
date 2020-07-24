package main

import (
	"sync/atomic"
	"time"
	"unsafe"
)

func init() {
	// Default time is UTC - set atomically.
	utc_time := unsafe.Pointer(time.UTC)
	local_time := (*unsafe.Pointer)(unsafe.Pointer(&time.Local))

	atomic.StorePointer(local_time, utc_time)
}
