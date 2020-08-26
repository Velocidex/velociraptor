package vql

import (
	"fmt"
	"sync"

	"www.velocidex.com/golang/vfilter"
)

type _destructors struct {
	mu sync.Mutex

	fn []func()
}

func AddGlobalDestructor(scope *vfilter.Scope, fn func()) {
	destructors_any := scope.GetContext("__destructors")
	destructors, ok := destructors_any.(*_destructors)
	if !ok {
		destructors = &_destructors{}
		scope.SetContext("__destructors", destructors)
	}

	destructors.mu.Lock()
	defer destructors.mu.Unlock()

	destructors.fn = append(destructors.fn, fn)
}

func CallGlobalDestructors(scope *vfilter.Scope) {
	destructors_any := scope.GetContext("__destructors")
	destructors, ok := destructors_any.(*_destructors)
	if ok {
		destructors.mu.Lock()
		defer destructors.mu.Unlock()

		fmt.Printf("Calling global destructors %v\n", destructors.fn)
		for _, fn := range destructors.fn {
			fn()
		}
	}
}
