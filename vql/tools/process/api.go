package process

import (
	"context"

	"www.velocidex.com/golang/vfilter"
)

type IProcessTracker interface {
	Get(ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool)
	Enrich(ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool)
	Processes(ctx context.Context, scope vfilter.Scope) []*ProcessEntry
	Children(ctx context.Context, scope vfilter.Scope, id string) []*ProcessEntry
	CallChain(ctx context.Context, scope vfilter.Scope, id string) []*ProcessEntry

	// Listen to the update stream from the tracker.
	Updates() chan *ProcessEntry
}
