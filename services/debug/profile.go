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

func GetProfileWriters() (result []ProfileWriterInfo) {
	mu.Lock()
	defer mu.Unlock()

	for _, i := range handlers {
		result = append(result, i)
	}

	return result
}

func WriteProfile(ctx context.Context, scope vfilter.Scope, output_chan chan vfilter.Row) {
	for _, w := range handlers {
		w.ProfileWriter(ctx, scope, output_chan)
	}
}
