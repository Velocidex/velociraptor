package utils

import (
	"sync"
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

	self.count++

	return func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		self.count--
		if self.count == 0 && self.completion != nil {
			self.completion()
		}
	}
}
