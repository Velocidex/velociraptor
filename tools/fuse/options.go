package fuse

import "sync"

type Options struct {
	// Map raw device names like \\.\C: -> C:
	MapDeviceNamesToLetters bool

	// Stip the : from the drive name C: -> C
	MapDriveNamesToLetters bool

	// If TRUE, allow all path characters except / which will be
	// escaped (This is correct on Linux). If false, escape Windows
	// illegal characters in paths.
	UnixCompatiblePathEscaping bool

	// Emulate the timestamps from some common artifacts (Such as
	// Windows.KapeFiles.Targets)
	EmulateTimestamps bool

	mu         sync.Mutex
	timestamps map[string]*Timestamps
}
