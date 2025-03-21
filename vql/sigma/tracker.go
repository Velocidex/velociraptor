package sigma

import (
	"context"
	"sync"

	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/vfilter"
)

var (
	Tracker = &SigmaTracker{
		current_contexts: make(map[uint64]*SigmaContext),
	}
)

type SigmaTracker struct {
	mu               sync.Mutex
	current_contexts map[uint64]*SigmaContext
}

func (self *SigmaTracker) Register(context *SigmaContext) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.current_contexts[context.id] = context
}

func (self *SigmaTracker) Unregister(context *SigmaContext) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.current_contexts, context.id)
}

func (self *SigmaTracker) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, ref := range self.current_contexts {
		ref.ProfileWriter(ctx, scope, output_chan)
	}
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "SigmaTracker",
		Description:   "Track current Sigma operations",
		ProfileWriter: Tracker.ProfileWriter,
		Categories:    []string{"Global", "VQL", "Plugins"},
	})
}
