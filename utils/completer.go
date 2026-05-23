package utils

import (
	"fmt"
	"runtime/debug"
	"sync"
)

var (
	// Set this to convert completion functions to synchronous calls.
	SyncCompleter = func() {
		fmt.Printf("SyncCompleter should never be called! %v",
			string(debug.Stack()))
	}

	// A nil completion that indicates background writing - improves
	// readability in call sites. This indicates that we do not want
	// to wait for a response, and can proceed writing in the
	// background.
	BackgroundWriter func()
)

const (
	DEBUG_COMPLETER = false
)

/*
  The Completer is a helper that is used to ensure a completion
  function is called when several asynchronous operations are
  finished. Only when all operations are done, the completion function
  will be called.

  Suppose we have a number of concurrent asynchronous operations we
  need to perform. This is the common usage pattern.

  func doStuff() {
    completer, closer := NewCompleter(func() {
      fmt.Printf("I am called once")
    })

    // The closer must be called after all sub-completer functions are
    // created with GetCompletionFunc()
    defer closer()

    err := db.SetSubjectWithCompletion(...., completer.GetCompletionFunc())
    ...
    err := db.SetSubjectWithCompletion(...., completer.GetCompletionFunc())
  }

  NOTE: As a special case, if the completer is created with
  SyncCompleter it means all completion functions will be synchronous.
*/

type Completer struct {
	mu         sync.Mutex
	count      int
	completion func()
}

// To avoid a race the API has changed to force the completer to close
// at an opportune time. The closer should be called **after** all the
// completion functions have been called:
//
// completer, closer := NewCompleter(completion)
// defer closer()
//
// ... Code using the completer getting new sub-completions with
// completer.GetCompletionFunc()
//
// When the closer() is called all GetCompletionFunc() have been
// called on the completer!
func NewCompleter(completion func()) (completer *Completer, closer func()) {
	res := &Completer{
		completion: completion,
	}

	// Do not actually call the sync completer.
	if CompareFuncs(completion, SyncCompleter) {
		return res, func() {}
	}

	return res, res.GetCompletionFunc()
}

func (self *Completer) GetCompletionFunc() func() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// If we wrap the SyncCompleter this means that **all** calls must
	// be synchronous.
	if CompareFuncs(self.completion, SyncCompleter) {
		return self.completion
	}

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
