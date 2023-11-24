package assets

import "sync"

var (
	once sync.Once
	done = make(chan bool)
)

func InitOnce() {
	once.Do(func() {
		defer close(done)

		// Call the Init function to unpack the assets
		Init()
	})

	// Wait for the function to run.
	<-done
}
