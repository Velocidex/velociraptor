package server_monitoring

import "sync"

type QueryTracer struct {
	mu              sync.Mutex
	current_queries map[string]bool
}

func (self *QueryTracer) Set(query string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.current_queries[query] = true
}

func (self *QueryTracer) Clear(query string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.current_queries, query)
}

func (self *QueryTracer) Dump() []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []string{}
	for k := range self.current_queries {
		result = append(result, k)
	}

	return result
}

func NewQueryTracer() *QueryTracer {
	return &QueryTracer{
		current_queries: make(map[string]bool),
	}
}
