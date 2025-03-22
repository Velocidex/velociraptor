package readers

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	Tracker = &NTFSCacheTracker{
		current_contexts: make(map[uint64]*NTFSCachedContext),
	}
)

type NTFSCacheTracker struct {
	mu               sync.Mutex
	current_contexts map[uint64]*NTFSCachedContext
}

func (self *NTFSCacheTracker) Register(context *NTFSCachedContext) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.current_contexts[context.id] = context
}

func (self *NTFSCacheTracker) Unregister(context *NTFSCachedContext) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.current_contexts, context.id)
}

func (self *NTFSCacheTracker) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, ref := range self.current_contexts {
		ref.ProfileWriter(ctx, scope, output_chan)
	}
}

func (self *NTFSCachedContext) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	next_refresh := ""
	if !self.next_refresh.IsZero() {
		next_refresh = self.next_refresh.Sub(utils.GetTime().Now()).
			Round(time.Second).String()
	}

	var mft_entries, cached_pages *ordereddict.Dict
	if self.paged_reader != nil {
		cached_pages = self.paged_reader.Stats()
	}

	if self.ntfs_ctx != nil {
		mft_entries = self.ntfs_ctx.Stats()
	}

	started := ""
	if !self.started.IsZero() {
		started = utils.GetTime().Now().Sub(self.started).
			Round(time.Second).String()
	}

	select {
	case <-ctx.Done():
		return

	case output_chan <- ordereddict.NewDict().
		Set("ID", self.id).
		Set("Active", self.ntfs_ctx != nil).
		Set("Accessor", self.accessor).
		Set("Device", self.device).
		Set("Started", started).

		// Next time we reset the NTFS cache.
		Set("NextRefresh", next_refresh).
		Set("PageCache", cached_pages).
		Set("MFTEntries", mft_entries):
	}

}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "NTFS Cache Tracker",
		Description:   "Track NTFS caches",
		ProfileWriter: Tracker.ProfileWriter,
		Categories:    []string{"Global", "VQL", "Plugins"},
	})
}
