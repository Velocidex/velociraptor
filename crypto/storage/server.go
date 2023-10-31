package storage

import "sync"

var (
	mu               sync.Mutex
	currentServerPEM []byte
)

func SetCurrentServerPem(pem []byte) {
	mu.Lock()
	defer mu.Unlock()

	currentServerPEM = pem
}
