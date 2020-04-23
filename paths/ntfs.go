package paths

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// For convenience we transform paths like c:\Windows -> \\.\c:\Windows
	driveRegex = regexp.MustCompile(
		`(?i)^[/\\]?([a-z]:)(.*)`)
	deviceDriveRegex = regexp.MustCompile(
		`(?i)^(\\\\[\?\.]\\[a-zA-Z]:)(.*)`)
	deviceDirectoryRegex = regexp.MustCompile(
		`(?i)^(\\\\[\?\.]\\GLOBALROOT\\Device\\[^/\\]+)([/\\]?.*)`)
)

func GetDeviceAndSubpath(path string) (device string, subpath string, err error) {
	// Make sure not to run filepath.Clean() because it will
	// collapse multiple slashes (and prevent device names from
	// being recognized).
	path = strings.Replace(path, "/", "\\", -1)

	m := deviceDriveRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return m[1], clean(m[2]), nil
	}

	m = driveRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return "\\\\.\\" + m[1], clean(m[2]), nil
	}

	m = deviceDirectoryRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return m[1], clean(m[2]), nil
	}

	return "/", path, errors.New("Unsupported device type")
}

func clean(path string) string {
	result := filepath.Clean(path)
	if result == "." {
		result = ""
	}

	return result
}
