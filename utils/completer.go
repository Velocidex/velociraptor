package utils

import (
	"sync"
)

type Completer struct {
	mu         sync.Mutex
	count      int
	completion func()
}

func NewCompleter(completion func()) *Completer {
	return &Completer{
		completion: completion,
	}
}

func (self *Completer) GetCompletionFunc() func() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.count++

	return func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		self.count--
		if self.count == 0 {
			self.completion()
		}
	}
}
