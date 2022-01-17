package api

import (
	"fmt"
	"runtime/debug"
)

var (
	// Set this to convert completion functions to synchronous calls.
	SyncCompleter = func() {
		fmt.Printf("SyncCompleter should never be called! %v",
			string(debug.Stack()))
	}
)
