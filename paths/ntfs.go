package paths

import (
	"errors"
	"regexp"
	"strings"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
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

func UnsafeDatastorePathFromClientPath(
	base_path api.PathSpec,
	accessor, client_path string) api.PathSpec {
	device, subpath, err := GetDeviceAndSubpath(client_path)
	if !utils.IsNil(base_path) {
		if err == nil {
			return base_path.AddUnsafeChild(
				accessor, device).AddChild(subpath...)
		}
		return base_path.AddUnsafeChild(accessor).AddChild(
			utils.SplitComponents(client_path)...)
	}
	return api.NewUnsafeDatastorePath(accessor).AddChild(
		utils.SplitComponents(client_path)...)
}

// Detect device names from a client's path.
func GetDeviceAndSubpath(path string) (device string, subpath []string, err error) {
	path = strings.Replace(path, "/", "\\", -1)

	m := deviceDriveRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return m[1], utils.SplitComponents(m[2]), nil
	}

	m = driveRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return "\\\\.\\" + m[1], utils.SplitComponents(m[2]), nil
	}

	m = deviceDirectoryRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return m[1], utils.SplitComponents(m[2]), nil
	}

	return "/", nil, errors.New("Unsupported device type")
}

func GetDeviceAndSubpathComponents(path_components []string) (device string, subpath_components []string, err error) {
	if len(path_components) == 0 {
		return "", nil, errors.New("Unsupported device type")
	}

	// Check the first component for a device spec
	m := deviceDriveRegex.FindStringSubmatch(path_components[0])
	if len(m) != 0 {
		return path_components[0], path_components[1:], nil
	}

	// Check it for a drive
	m = driveRegex.FindStringSubmatch(path_components[0])
	if len(m) != 0 {
		return "\\\\.\\" + m[1], path_components[1:], nil
	}

	m = deviceDirectoryRegex.FindStringSubmatch(path_components[0])
	if len(m) != 0 {
		return m[1], path_components[1:], nil
	}

	return "/", path_components, errors.New("Unsupported device type")
}
