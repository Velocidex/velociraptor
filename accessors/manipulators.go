package accessors

import (
	"regexp"
	"strings"

	"www.velocidex.com/golang/velociraptor/utils"
)

// Responsible for serialization of linux paths
type LinuxPathManipulator int

func (self LinuxPathManipulator) PathSplit(path string) []string {
	components := strings.Split(path, "/")
	result := make([]string, 0, len(components))
	for _, c := range components {
		if c == "" || c == "." || c == ".." {
			continue
		}
		result = append(result, c)
	}

	return result
}

func (self LinuxPathManipulator) PathJoin(components []string) string {
	return "/" + strings.Join(components, "/")
}

func NewLinuxOSPath(path string) *OSPath {
	manipulator := LinuxPathManipulator(0)
	return &OSPath{
		Components:  manipulator.PathSplit(path),
		Manipulator: manipulator,
	}
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

func (self WindowsPathManipulator) PathSplit(path string) []string {
	m := deviceDriveRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return append([]string{m[1]}, utils.SplitComponents(m[2])...)
	}

	m = deviceDirectoryRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return append([]string{m[1]}, utils.SplitComponents(m[2])...)
	}

	return utils.SplitComponents(path)
}

func (self WindowsPathManipulator) PathJoin(components []string) string {
	// The first component is usually the drive letter or device and
	// although it can contain path separators it must not be quoted
	if len(components) == 0 {
		return ""
	}

	// No leading \\ as first component is drive letter
	return components[0] + utils.JoinComponents(components[1:], "\\")
}

func NewWindowsOSPath(path string) *OSPath {
	manipulator := WindowsPathManipulator(0)
	return &OSPath{
		Components:  manipulator.PathSplit(path),
		Manipulator: manipulator,
	}
}
