package main

import "time"

func init() {
	// Default time is UTC - set atomically.
	time.Local = time.UTC
}
