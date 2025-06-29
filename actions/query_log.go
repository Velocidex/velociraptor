/*

Keeps an in memory log of recent queries that can be used for
debugging the client.

*/

package actions

import (
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	QueryLog = NewQueryLog()
)

type QueryLogEntry struct {
	mu       sync.Mutex
	Query    string
	Start    time.Time
	Duration int64
}

func (self *QueryLogEntry) Copy() *QueryLogEntry {
	self.mu.Lock()
	defer self.mu.Unlock()

	return &QueryLogEntry{
		Query:    self.Query,
		Start:    self.Start,
		Duration: self.Duration,
	}
}

func (self *QueryLogEntry) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Query was already closed - allow Close to be called multiple
	// times.
	if self.Duration > 0 {
		return
	}

	self.Duration = utils.Now().UnixNano() - self.Start.UnixNano()

	// We represent Duration == 0 as not yet complete but sometimes
	// the query is closed so fast that self.Duration above is still
	// zero. Account for this and make it 1.
	if self.Duration == 0 {
		self.Duration = 1
	}
}

type QueryLogType struct {
	mu sync.Mutex

	limit int

	Queries []*QueryLogEntry
}

func (self *QueryLogType) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.Queries = nil
}

func (self *QueryLogType) AddQuery(query string) *QueryLogEntry {
	self.mu.Lock()
	defer self.mu.Unlock()

	q := &QueryLogEntry{
		Query: query,
		Start: utils.Now(),
	}

	self.Queries = append(self.Queries, q)

	if len(self.Queries) > self.limit {
		// Drop the first finished message. This should keep the
		// queries that are in flight in the queue as much as
		// possible.
		dropped := false
		new_queries := make([]*QueryLogEntry, 0, len(self.Queries))
		for _, i := range self.Queries {
			if !dropped && i.Duration != 0 {
				dropped = true
			} else {
				new_queries = append(new_queries, i)
			}
		}
		self.Queries = new_queries
	}

	return q
}

func (self *QueryLogType) Get() []*QueryLogEntry {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Return a copy of the logs
	result := make([]*QueryLogEntry, 0, len(self.Queries))
	for _, q := range self.Queries {
		result = append(result, q.Copy())
	}

	return result
}

func NewQueryLog() *QueryLogType {
	return &QueryLogType{
		limit: 100,
	}
}
