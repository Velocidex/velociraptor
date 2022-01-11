package paths

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	mappingPathRegex = regexp.MustCompile(`(?i)^\\\\.\\([^\\]+)(.*)$`)
)

func findRoot(path string) (root, subpath string) {
	path = filepath.Clean(path)

	// strip first path separator - we assume absolute paths anyway
	if strings.HasPrefix(path, "\\") {
		return "\\", path[1:]
	}

	return "", path
}

func ConvertPathToRemappedPath(path string) (device, subpath string, err error) {
	// check if we have a mapping path already
	// \\.\<device>\path\to\file -> \\.\<device> and \path\to\file

	m := mappingPathRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		device := m[1]
		_, subpath := findRoot(m[2])
		return device, subpath, nil
	}

	_, subpath = findRoot(path)
	return "*", subpath, errors.New("cannot determine device from path")

}
