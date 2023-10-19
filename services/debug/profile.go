package debug

import (
	"context"

	"www.velocidex.com/golang/vfilter"
)

type ProfileWriter func(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row)

var (
	handlers []ProfileWriter
)

func RegisterProfileWriter(writer ProfileWriter) {
	handlers = append(handlers, writer)
}

func WriteProfile(ctx context.Context, scope vfilter.Scope, output_chan chan vfilter.Row) {
	for _, w := range handlers {
		w(ctx, scope, output_chan)
	}
}
