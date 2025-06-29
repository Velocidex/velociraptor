//go:build !windows
// +build !windows

package http_comms

import (
	"os"
	"path/filepath"
)

// Reset the buffer file by removing old data. We prevent symlink
// attacks by replacing any existing file with a new file. In this
// case we do not want to use a random file name because the
// Velociraptor client is often killed without warning and
// restarted (e.g. system reboot). This means we dont always get a
// chance to cleanup and after a lot of restarts random file names
// will accumulate.

// By default the temp directory is created inside a protected
// directory (`C:\Program Files\Velociraptor\Tools`) so symlink
// attacks are mitigated but in case Velociraptor is misconfigured we
// are extra careful.
func createFile(filename string) (*os.File, error) {

	// Strategy on Linux is to create a tempfile and move it into
	// position. This will remove the old file, even if another
	// instance is holding it open.
	basename := filepath.Base(filename)
	fd, err := os.CreateTemp(filepath.Dir(filename), basename)
	if err != nil {
		return nil, err
	}

	// In tests the file has to be discoverable so we can read it -
	// move to its known position.
	if PREPARE_FOR_TESTS {
		err := os.Rename(fd.Name(), filename)
		return fd, err
	}

	// Try to just remove the file while keeping the file handle open
	// - this should work on most unix like operating systems..
	err = os.Remove(fd.Name())
	if err == nil {
		return fd, err
	}

	// If we can not remove it, move it into position so we dont leave
	// tempfiles behind.
	err = os.Rename(fd.Name(), filename)
	if err != nil {
		// If that didnt work, we just give up. Close the file, then
		// delete it.
		fd.Close()
		os.Remove(fd.Name())

		return nil, err
	}

	return fd, err
}
