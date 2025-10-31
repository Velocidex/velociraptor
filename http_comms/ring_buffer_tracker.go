package http_comms

import (
	"context"
	"sort"
	"sync"

	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/vfilter"
)

var (
	Tracker = &LocalBufferTracker{
		tracked: make(map[uint64]IRingBuffer),
	}
)

type LocalBufferTracker struct {
	mu sync.Mutex

	tracked map[uint64]IRingBuffer
}

func (self *LocalBufferTracker) Register(id uint64, rb IRingBuffer) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.tracked[id] = rb
	if rb == nil {
		delete(self.tracked, id)
	}
}

func (self *LocalBufferTracker) ProfileWriter(ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row) {
	self.mu.Lock()
	defer self.mu.Unlock()

	var ids []uint64
	for id := range self.tracked {
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	for _, id := range ids {
		rb, _ := self.tracked[id]
		rb.ProfileWriter(ctx, scope, output_chan)
	}
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "Local Buffer File",
		Description:   "Track statistics about local buffer file",
		ProfileWriter: Tracker.ProfileWriter,
		Categories:    []string{"Client"},
	})
}
