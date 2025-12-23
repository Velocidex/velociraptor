package directory

import (
	"strings"

	"www.velocidex.com/golang/velociraptor/utils"
)

// Check for directory traversal sequences. These should never happen
// but we have a second layer of defence here.
func checkPath(path string) error {
	if strings.Contains(path, "/../") {
		return utils.Wrap(utils.InvalidArgError, "Directory traversal not supported")
	}

	return nil
}
