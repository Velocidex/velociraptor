package hunt_dispatcher

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
)

type HuntRefreshStats struct {
	sync.Mutex

	Type                string
	Time                time.Time
	Duration            time.Duration
	TotalHunts          uint64
	TotalFlowsInspected uint64
	TotalFlows          uint64
}

type HuntDispatcherTracker struct {
	mu sync.Mutex

	RecentRefresh []*HuntRefreshStats
}

func (self *HuntDispatcherTracker) AddRefreshStats(s *HuntRefreshStats) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.RecentRefresh = append(self.RecentRefresh, s)
	if len(self.RecentRefresh) > 10 {
		self.RecentRefresh = self.RecentRefresh[len(self.RecentRefresh)-10:]
	}
}

func (self *HuntDispatcherTracker) WriteProfile(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row) {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, item := range self.RecentRefresh {
		output_chan <- ordereddict.NewDict().
			Set("Type", item.Type).
			Set("Time", item.Time.UTC().Format(time.RFC3339)).
			Set("Ago", time.Now().Sub(item.Time).Round(time.Second).String()).
			Set("Duration", item.Duration.Round(time.Second).String()).
			Set("TotalHunts", item.TotalHunts).
			Set("TotalFlows", item.TotalFlows).
			Set("TotalFlowsInspected", item.TotalFlowsInspected)
	}
}
