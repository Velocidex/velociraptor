package backup

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type BackupTrackerStats struct {
	Timestamp time.Time
	Filename  string
	Stats     []services.BackupStat
}

func NewBackupTrackerStats(export_path api.FSPathSpec) *BackupTrackerStats {
	return &BackupTrackerStats{
		Timestamp: utils.GetTime().Now(),
		Filename:  export_path.String(),
	}
}

func (self *BackupService) ProfileWriter(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	display_time := func(t time.Time) string {
		return t.UTC().Round(time.Second).Format(time.RFC3339)
	}

	for _, r := range self.registrations {
		output_chan <- ordereddict.NewDict().
			Set("Prvovider", r.ProviderName()).
			Set("Name", r.Name())
	}

	now := utils.GetTime().Now()

	output_chan <- ordereddict.NewDict().
		Set("Delay", self.delay.Round(time.Second).String()).
		Set("LastRun", display_time(self.last_run)).
		Set("NextRun", display_time(self.last_run.Add(self.delay))).
		Set("NextRunIn", self.last_run.Add(self.delay).Sub(now).
			Round(time.Second).String())

	for _, s := range self.stats {
		output_chan <- ordereddict.NewDict().
			Set("Timestamp", display_time(s.Timestamp)).
			Set("Filename", s.Filename).
			Set("Stats", s.Stats)
	}
}
