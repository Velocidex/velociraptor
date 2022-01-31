package accessors

import (
	"regexp"
	"strings"

	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Responsible for serialization of linux paths
type LinuxPathManipulator int

func (self LinuxPathManipulator) PathParse(path string, result *OSPath) {
	maybeParsePathSpec(path, result)
	path = result.pathspec.Path

	components := strings.Split(path, "/")
	result.Components = make([]string, 0, len(components))
	for _, c := range components {
		if c == "" || c == "." || c == ".." {
			continue
		}
		result.Components = append(result.Components, c)
	}
}

func (self LinuxPathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	}
	result.Path = "/" + strings.Join(path.Components, "/")
	return result
}

func (self LinuxPathManipulator) PathJoin(path *OSPath) string {
	return self.AsPathSpec(path).Path
}

func NewLinuxOSPath(path string) *OSPath {
	manipulator := LinuxPathManipulator(0)
	result := &OSPath{
		pathspec:    &PathSpec{},
		Manipulator: manipulator,
	}

	manipulator.PathParse(path, result)

	return result
}

var (
	// For convenience we transform paths like c:\Windows -> \\.\c:\Windows
	driveRegex = regexp.MustCompile(
		`(?i)^[/\\]?([a-z]:)(.*)`)

	deviceDriveRegex = regexp.MustCompile(
		`(?i)^(\\\\[\?\.]\\[a-zA-Z]:)(.*)`)

	deviceDirectoryRegex = regexp.MustCompile(
		`(?i)^(\\\\[\?\.]\\GLOBALROOT\\Device\\[^/\\]+)([/\\]?.*)`)
)

// Breaks a client path into components. The client's path may consist
// of a drive letter or a device which will be treated as a single
// component. For example:
// C:\Windows -> "C:\", "Windows"
// \\.\c:\Windows -> "\\.\C:", "Windows"

// Other components that contain path separators need to be properly
// quoted as usual:
// HKEY_LOCAL_MACHINE\Software\Microsoft\"http://www.google.com"\Foo ->
// "HKEY_LOCAL_MACHINE", "Software", "Microsoft", "http://www.google.com", "Foo"

type WindowsPathManipulator int

func (self WindowsPathManipulator) PathParse(path string, result *OSPath) {
	maybeParsePathSpec(path, result)
	path = result.pathspec.Path

	m := deviceDriveRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		result.Components = append([]string{m[1]}, utils.SplitComponents(m[2])...)
		return
	}

	m = deviceDirectoryRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		result.Components = append([]string{m[1]}, utils.SplitComponents(m[2])...)
		return
	}

	result.Components = utils.SplitComponents(path)
}

func (self WindowsPathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	}
	result.Path = self.PathJoin(path)
	return result
}

func (self WindowsPathManipulator) PathJoin(path *OSPath) string {
	// The first component is usually the drive letter or device and
	// although it can contain path separators it must not be quoted
	components := path.Components

	if len(components) == 0 {
		return ""
	}

	// No leading \\ as first component is drive letter
	return components[0] + utils.JoinComponents(components[1:], "\\")
}

func NewWindowsOSPath(path string) *OSPath {
	manipulator := WindowsPathManipulator(0)
	result := &OSPath{
		Manipulator: manipulator,
	}
	manipulator.PathParse(path, result)

	return result
}

type PathSpecPathManipulator int

func (self PathSpecPathManipulator) PathParse(path string, result *OSPath) {
	pathspec, err := PathSpecFromString(path)
	if err == nil {
		result.pathspec = pathspec
		// Break the path into components
		result.Components = utils.SplitComponents(pathspec.Path)
		return
	}

	result.pathspec = &PathSpec{Path: path}
}

func (self PathSpecPathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	}
	result.Path = "/" + strings.Join(path.Components, "/")
	return result
}

func (self PathSpecPathManipulator) PathJoin(path *OSPath) string {
	result := "/" + strings.Join(path.Components, "/")
	if path.pathspec != nil {
		path.pathspec.Path = result
		return path.pathspec.String()
	}

	return result
}

func NewPathspecOSPath(path string) *OSPath {
	manipulator := PathSpecPathManipulator(0)
	result := &OSPath{
		Manipulator: manipulator,
	}

	manipulator.PathParse(path, result)
	return result
}

func maybeParsePathSpec(path string, result *OSPath) {
	if strings.HasPrefix(path, "{") {
		pathspec := &PathSpec{}
		err := json.Unmarshal([]byte(path), pathspec)
		if err == nil {
			result.pathspec = pathspec
			return
		}
	}

	result.pathspec = &PathSpec{
		Path: path,
	}
}
