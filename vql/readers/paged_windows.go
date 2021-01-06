// +build windows

package readers

// Windows needs to load the file accessor for the test
import (
	_ "www.velocidex.com/golang/velociraptor/vql/windows/filesystems"
)
