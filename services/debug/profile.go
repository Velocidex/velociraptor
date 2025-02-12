package debug

import (
	"context"
	"sync"

	"www.velocidex.com/golang/vfilter"
)

type ProfileWriter func(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row)

type ProfileWriterInfo struct {
	Name, Description string
	ProfileWriter     ProfileWriter
	ID                uint64
}

var (
	mu       sync.Mutex
	handlers []ProfileWriterInfo
)

func RegisterProfileWriter(writer ProfileWriterInfo) {
	mu.Lock()
	defer mu.Unlock()

	handlers = append(handlers, writer)
}

func UnregisterProfileWriter(id uint64) {
	mu.Lock()
	defer mu.Unlock()

	new_handlers := make([]ProfileWriterInfo, 0, len(handlers))
	for _, h := range handlers {
		if h.ID != id {
			new_handlers = append(new_handlers, h)
		}
	}
	handlers = new_handlers
}

func GetProfileWriters() (result []ProfileWriterInfo) {
	mu.Lock()
	defer mu.Unlock()

	for _, i := range handlers {
		result = append(result, i)
	}

	return result
}
