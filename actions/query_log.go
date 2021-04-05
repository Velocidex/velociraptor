package actions

import (
	"sync"
	"time"
)

var (
	QueryLog = NewQueryLog()
)

type QueryLogEntry struct {
	Query    string
	Start    time.Time
	Duration int64
}

func (self *QueryLogEntry) Close() {
	self.Duration = time.Now().UnixNano() - self.Start.UnixNano()
}

type QueryLogType struct {
	mu sync.Mutex

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
		Start: time.Now(),
	}

	self.Queries = append(self.Queries, q)

	if len(self.Queries) > 50 {
		self.Queries = self.Queries[1:]
	}

	return q
}

func (self *QueryLogType) Get() []QueryLogEntry {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Return a copy of the logs
	result := make([]QueryLogEntry, 0, len(self.Queries))
	for _, q := range self.Queries {
		result = append(result, *q)
	}

	return result
}

func NewQueryLog() *QueryLogType {
	return &QueryLogType{}
}
