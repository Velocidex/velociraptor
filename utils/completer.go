package utils

import (
	"fmt"
	"runtime/debug"
	"sync"
)

const (
	DEBUG_COMPLETER = false
)

/*
  The Completer is a helper that is used to ensure a completion
  funciton is called when several asynchronous operations are
  finished. Only when all operations are done, the completion function
  will be called.

  Suppose we have a number of concurrent asynchronous operations we
  need to perform. This is the common usage pattern.

  func doStuff() {
    completer := NewCompleter(func() {
      fmt.Printf("I am called once")
    })

    // This ensures the completer is not called until we leave this
    // function.
    defer completer.GetCompletionFunc()()

    err := db.SetSubjectWithCompletion(...., completer.GetCompletionFunc())
    ...
    err := db.SetSubjectWithCompletion(...., completer.GetCompletionFunc())
  }

*/

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

	id := self.count
	self.count++

	var stack string

	if DEBUG_COMPLETER {
		stack = string(debug.Stack())
		fmt.Printf("Adding completion %v: %v \n", id, stack)
	}

	return func(id int, stack string) func() {
		return func() {
			self.mu.Lock()
			defer self.mu.Unlock()

			if DEBUG_COMPLETER {
				fmt.Printf("Removing completion %v: %v \n", id, stack)
			}

			self.count--
			if self.count == 0 && self.completion != nil {
				if DEBUG_COMPLETER {
					fmt.Printf("Firing!!!!\n")
				}
				self.completion()
			}
		}
	}(id, stack)
}
