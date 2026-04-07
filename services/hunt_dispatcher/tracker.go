package hunt_dispatcher

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// Stats about hunt refresh
type HuntRefreshStats struct {
	sync.Mutex

	Type                string
	Time                time.Time
	Duration            time.Duration
	TotalHunts          uint64
	TotalFlowsInspected uint64
	TotalHuntsSkipped   uint64
	TotalFlows          uint64
	CurrentHunts        *ordereddict.Dict
}

func (self *HuntRefreshStats) ToDict() *ordereddict.Dict {
	self.Lock()
	defer self.Unlock()

	return ordereddict.NewDict().
		Set("Type", self.Type).
		Set("Time", self.Time.UTC().Format(time.RFC3339)).
		Set("Ago", time.Now().Sub(self.Time).Round(time.Second).String()).
		Set("Duration", self.Duration.Round(time.Second).String()).
		Set("TotalHunts", self.TotalHunts).
		Set("TotalHuntsSkipped", self.TotalHuntsSkipped).
		Set("TotalFlows", self.TotalFlows).
		Set("TotalFlowsInspected", self.TotalFlowsInspected).
		Set("CurrentHunts", self.CurrentHunts)
}

func NewHuntRefreshStats(name string) *HuntRefreshStats {
	return &HuntRefreshStats{
		Type:         name,
		Time:         utils.GetTime().Now(),
		CurrentHunts: ordereddict.NewDict(),
	}
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
	var refreshes []*HuntRefreshStats
	for _, item := range self.RecentRefresh {
		refreshes = append(refreshes, item)
	}
	self.mu.Unlock()

	for _, item := range refreshes {
		output_chan <- item.ToDict()
	}
}
