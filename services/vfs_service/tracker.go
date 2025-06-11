package vfs_service

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type VFSServiceStats struct {
	start, end         time.Time
	client_id, flow_id string
	rows, directories  int
}

func (self *VFSServiceStats) Close() {
	self.end = utils.GetTime().Now()
}

func (self *VFSServiceStats) ChargeDir(rows int) {
	self.rows += rows
	self.directories += 1
}

func (self *VFSServiceStats) Stats() *ordereddict.Dict {
	duration := "Processing"
	if !self.end.IsZero() {
		duration = self.end.Sub(self.start).Round(time.Second).String()
	}

	return ordereddict.NewDict().
		Set("ClientId", self.client_id).
		Set("FlowId", self.flow_id).
		Set("StartedAgo", utils.GetTime().Now().
			Sub(self.start).Round(time.Second).String()).
		Set("Duration", duration).
		Set("TotalDirectories", self.directories).
		Set("TotalFilesListed", self.rows)
}

func (self *VFSService) WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	var rows []*ordereddict.Dict

	self.mu.Lock()
	for _, s := range self.stats {
		rows = append(rows, s.Stats())
	}

	if self.current_stat != nil {
		rows = append(rows, self.current_stat.Stats())
	}
	self.mu.Unlock()

	for _, r := range rows {
		select {
		case <-ctx.Done():
			return

		case output_chan <- r:
		}
	}
}

func (self *VFSService) startNewOperation(client_id, flow_id string) func() {
	self.mu.Lock()
	defer self.mu.Unlock()
	if self.current_stat != nil {
		self.stats = append(self.stats, self.current_stat)

		for len(self.stats) > 50 {
			self.stats = self.stats[1:]
		}
	}
	self.current_stat = &VFSServiceStats{
		start:     utils.GetTime().Now(),
		client_id: client_id,
		flow_id:   flow_id,
	}

	return self.current_stat.Close
}
