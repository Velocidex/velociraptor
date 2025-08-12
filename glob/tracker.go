package glob

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	globTracker = GlobTracker{
		globs: make(map[uint64]*RootGlobber),
	}
)

type Action struct {
	time       time.Time
	path       *accessors.OSPath
	accessor   string
	file_count int
}

type Actions struct {
	len, idx int

	actions [10]Action
}

func (self *Actions) Push(action Action) {
	cap := len(self.actions)

	self.idx = (self.idx + 1) % cap

	action.time = utils.GetTime().Now()
	self.actions[self.idx] = action

	if self.len < cap {
		self.len++
	}
}

func (self *Actions) Get() []Action {
	cap := len(self.actions)
	res := make([]Action, 0, cap)

	// Simple loop in modulo cap

	// Make sure its always positive
	i := (self.idx - self.len + cap) % cap
	for {
		next := (i + 1) % cap

		// We got to the current pointer.
		if next == self.idx {
			break
		}

		res = append(res, self.actions[i])
		i = next
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].time.Before(res[j].time)
	})
	return res
}

func (self *Globber) recordDirectory(
	path *accessors.OSPath, accessor string,
	file_count int) {
	if self.root != nil && path != nil {
		self.root.mu.Lock()
		defer self.root.mu.Unlock()

		self.root.last_dir.Push(Action{
			path:       path,
			accessor:   accessor,
			file_count: file_count,
		})
	}
}

type GlobTracker struct {
	mu    sync.Mutex
	globs map[uint64]*RootGlobber
}

func (self *GlobTracker) Register(root *RootGlobber) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.globs[root.id] = root
}

func (self *GlobTracker) Unregister(root *RootGlobber) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.globs, root.id)
}

func (self *GlobTracker) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, ref := range self.globs {
		ref.mu.Lock()
		actions := ref.last_dir.Get()
		ref.mu.Unlock()

		now := utils.GetTime().Now()

		for _, action := range actions {
			if action.path == nil {
				continue
			}

			row := ordereddict.NewDict().
				Set("GlobberId", ref.id).
				Set("Time", action.time).
				Set("Age", now.Sub(action.time).String()).
				Set("Dir", action.path.String()).
				Set("DirCount", action.file_count).
				Set("Accessor", action.accessor)
			output_chan <- row
		}
	}
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "GlobTracker",
		Description:   "Track current Glob operations",
		ProfileWriter: globTracker.ProfileWriter,
		Categories:    []string{"Global", "VQL", "Plugins"},
	})
}
