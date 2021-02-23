package responder

import "sync"

type PoolResponder struct {
	mu sync.Mutex
}
