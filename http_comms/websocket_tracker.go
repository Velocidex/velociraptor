package http_comms

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
	wsTracker = &WebsocketTracker{
		connections: make(map[string]*Conn),
	}
)

type ConnStats struct {
	mu sync.Mutex

	id                         uint64
	create_time, read_deadline time.Time
	write_deadline, last_ping  time.Time

	read_locked, write_locked bool
}

func (self *ConnStats) SetReadLock(state bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.read_locked = state
}

func (self *ConnStats) SetWriteLock(state bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.write_locked = state
}

func (self *ConnStats) SetReadDeadline(deadline time.Time) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.read_deadline = deadline
}

func (self *ConnStats) SetWriteDeadline(deadline time.Time) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.write_deadline = deadline
}

func (self *ConnStats) SetLastPing(now time.Time) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_ping = now
}

func (self *ConnStats) Get() *ConnStats {
	self.mu.Lock()
	defer self.mu.Unlock()

	return &ConnStats{
		id:             self.id,
		read_deadline:  self.read_deadline,
		write_deadline: self.write_deadline,
		last_ping:      self.last_ping,
		read_locked:    self.read_locked,
		write_locked:   self.write_locked,
		create_time:    self.create_time,
	}
}

type WebsocketTracker struct {
	mu          sync.Mutex
	connections map[string]*Conn
}

func (self *WebsocketTracker) Register(key string, conn *Conn) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.connections[key] = conn
}

func (self *WebsocketTracker) Unregister(key string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.connections, key)
}

func (self *WebsocketTracker) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, k := range utils.Sort(self.connections) {
		v := self.connections[k]
		v.ProfileWriter(ctx, scope, output_chan)
	}
}

func (self *Conn) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {
	now := utils.GetTime().Now()

	// Get a copy of the stats so we dont block the actual connection.
	stats := self.stats.Get()

	display := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}

		return now.Sub(t).Round(time.Second).String()
	}

	output_chan <- ordereddict.NewDict().
		Set("Key", self.key).
		Set("Age", display(stats.create_time)).
		Set("ReadDeadline", display(stats.read_deadline)).
		Set("WriteDeadline", display(stats.write_deadline)).
		Set("LastPing", display(stats.last_ping)).
		Set("ReadLocked", stats.read_locked).
		Set("WrtieLocked", stats.write_locked)
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "WS Connections",
		Description:   "Track websocket connections",
		ProfileWriter: wsTracker.ProfileWriter,
		Categories:    []string{"Global"},
	})
}
