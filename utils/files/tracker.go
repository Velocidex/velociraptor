// This metric tracks things that should be open and closed (like
// e.g. files).

// When the object is created, we call Add() and when it is closed we
// call Remove(). The tracker records the last few minutes of
// Add/Remove pairs so it is easy to check at various points in the
// program if Adds are perfectly paired with Removes.

package files

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	gTracker *OpenerTracker
)

type Opener struct {
	name               string
	callsite_added     string
	callsite_removed   string
	created, destroyed time.Time
	age                time.Time
}

type OpenerTracker struct {
	mu    sync.Mutex
	items map[string]*Opener
}

func (self *OpenerTracker) scan() {
	// Keep some recent files around for a max of 1 min, but always
	// keep tmpfiles that are in use.
	if len(self.items) > 20 {
		now := utils.Now()
		cutoff := now.Add(-time.Minute)

		var expired []string
		for k, v := range self.items {
			if !v.destroyed.IsZero() &&
				v.age.Before(cutoff) {
				expired = append(expired, k)
			}
		}

		for _, k := range expired {
			delete(self.items, k)
		}
	}

	// Something went wrong! avoid memory leaks.
	if len(self.items) > 1000 {
		self.items = make(map[string]*Opener)
	}
}

func (self *OpenerTracker) AddFile(filename string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	callsite := ""
	_, file, no, ok := runtime.Caller(2)
	if ok {
		callsite = fmt.Sprintf("%s#%d", file, no)
	}

	self.items[filename] = &Opener{
		name:           filename,
		created:        utils.Now(),
		callsite_added: callsite,
	}
	self.scan()
}

func (self *OpenerTracker) RemoveFile(filename string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	callsite := ""
	_, file, no, ok := runtime.Caller(2)
	if ok {
		callsite = fmt.Sprintf("%s#%d", file, no)
	}

	record, pres := self.items[filename]
	if !pres {
		record = &Opener{
			name: filename,
		}
	}

	now := utils.Now()
	record.destroyed = now
	record.age = now
	record.callsite_removed = callsite

	self.items[filename] = record

	self.scan()
}

func (self *OpenerTracker) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {
	self.mu.Lock()
	items := []*Opener{}
	for _, i := range self.items {
		items = append(items, i)
	}
	self.mu.Unlock()

	for _, v := range items {
		select {
		case <-ctx.Done():
			return

		case output_chan <- ordereddict.NewDict().
			Set("Name", v.name).
			Set("AddedFrom", v.callsite_added).
			Set("RemovedFrom", v.callsite_removed).
			Set("Created", v.created).
			Set("Destroyed", v.destroyed):
		}
	}
}

func Add(filename string) {
	gTracker.AddFile(filename)
}

func Remove(filename string) {
	gTracker.RemoveFile(filename)
}

func init() {
	gTracker = &OpenerTracker{
		items: make(map[string]*Opener),
	}

	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "open_close",
		Description:   "Track open items that should be closed.",
		ProfileWriter: gTracker.ProfileWriter,
		Categories:    []string{"Global", "Services"},
	})

}
