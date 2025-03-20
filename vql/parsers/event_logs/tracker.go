package event_logs

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	vfilter "www.velocidex.com/golang/vfilter"
)

type EventLogWatcherStats struct {
	Filename      string
	FindLastEvent int64 // Nanoseconds
	MonitorOnce   int64
	Count         int64
	FirstScan     time.Time
	LastScan      time.Time
	NextScan      time.Time
}

type EventLogWatcherTracker struct {
	mu    sync.Mutex
	files map[string]*EventLogWatcherStats
}

func (self *EventLogWatcherTracker) AddRow(
	filename *accessors.OSPath, accessor_name string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	key := accessor_name + filename.String()
	stats, pres := self.files[key]
	if !pres {
		return
	}

	stats.Count++
}

func (self *EventLogWatcherTracker) SetNextScan(
	filename *accessors.OSPath, accessor_name string, next time.Time) {
	self.mu.Lock()
	defer self.mu.Unlock()

	key := accessor_name + filename.String()
	stats, pres := self.files[key]
	if !pres {
		return
	}

	stats.NextScan = next
}

func (self *EventLogWatcherTracker) ChargeMonitorOnce(
	filename *accessors.OSPath, accessor_name string) func() {

	start := utils.GetTime().Now()
	key := accessor_name + filename.String()

	return func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		duration := utils.GetTime().Now().Sub(start)
		stats, pres := self.files[key]
		if !pres {
			stats = &EventLogWatcherStats{
				Filename: filename.String(),
			}
			self.files[key] = stats
		}

		stats.MonitorOnce += int64(duration)
		stats.LastScan = start
	}
}

func (self *EventLogWatcherTracker) ChargeFindLastEvent(
	filename *accessors.OSPath, accessor_name string) func() {

	start := utils.GetTime().Now()
	key := accessor_name + filename.String()

	return func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		duration := utils.GetTime().Now().Sub(start)

		stats, pres := self.files[key]
		if !pres {
			stats = &EventLogWatcherStats{
				Filename: filename.String(),
			}
			self.files[key] = stats
		}

		stats.FindLastEvent += int64(duration)
		stats.FirstScan = start
	}
}

func (self *EventLogWatcherTracker) WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	var keys []string
	for k := range self.files {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		stat, pres := self.files[k]
		if !pres {
			continue
		}

		output_chan <- ordereddict.NewDict().
			Set("Filename", stat.Filename).
			Set("FirstScan", stat.FirstScan).
			Set("LastScan", stat.LastScan).
			Set("NextScan", stat.NextScan.Sub(utils.GetTime().Now()).String()).
			Set("FindLastEvent", time.Duration(stat.FindLastEvent).String()).
			Set("MonitorOnce", time.Duration(stat.MonitorOnce).String()).
			Set("Count", stat.Count)
	}
}

func NewEventLogWatcherTracker() *EventLogWatcherTracker {
	return &EventLogWatcherTracker{
		files: make(map[string]*EventLogWatcherStats),
	}
}

var (
	eventLogWatchTracker *EventLogWatcherTracker = NewEventLogWatcherTracker()
)

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "Windows Event Log Watcher",
		Description:   "Records Statistics about the Windows Event Log Watcher Subsystem.",
		ProfileWriter: eventLogWatchTracker.WriteProfile,
		Categories:    []string{"Global", "VQL", "Plugins"},
	})

}
