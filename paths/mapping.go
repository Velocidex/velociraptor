package paths

import (
	"errors"
	"regexp"
)

var (
	mappingPathRegex = regexp.MustCompile(`(?i)^\\\\.\\([^\\]+)(.*)$`)
)

func ConvertPathToRemappedPath(path string) (device string, subpath string, err error) {
	// check if we have a mapping path already
	// \\.\<device>\path\to\file -> \\.\<device> and \path\to\file

	m := mappingPathRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return m[1], m[2], nil
	}

	return "*", path, errors.New("cannot determine device from path")

}
