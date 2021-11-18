package utils

import (
	"fmt"
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

	fmt.Printf("Getting completion %v: %v\n", self.count, self.completion)

	return func() {
		self.mu.Lock()
		defer self.mu.Unlock()
		self.count--
		fmt.Printf("Completed completion %v: %v\n", self.count, self.completion)
		if self.count == 0 {
			self.completion()
		}
	}
}
