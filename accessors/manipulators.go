package accessors

import (
	"net/url"
	"regexp"
	"strings"

	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

// This is a generic Path manipulator that implements the escaping
// standard as used by Velociraptor:
// 1. Path separators are / but will be able to use \\ to parse.
// 2. Each component is optionally quoted if it contains special
//    characters (like path separators).
type GenericPathManipulator struct{}

func (self GenericPathManipulator) PathParse(path string, result *OSPath) {
	maybeParsePathSpec(path, result)
	result.Components = utils.SplitComponents(result.pathspec.Path)
}

func (self GenericPathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	// Make a copy of the pathspec.
	var result PathSpec
	if path.pathspec != nil {
		result = *path.pathspec
	}

	// The first component is usually the drive letter or device and
	// although it can contain path separators it must not be quoted
	components := path.Components

	result.Path = utils.JoinComponents(components, "/")
	return &result
}

func (self GenericPathManipulator) PathJoin(path *OSPath) string {
	result := self.AsPathSpec(path)
	if result.GetDelegateAccessor() == "" && result.GetDelegatePath() == "" {
		return result.Path
	}
	return result.String()
}

func NewGenericOSPath(path string) *OSPath {
	manipulator := GenericPathManipulator{}
	result := &OSPath{
		Manipulator: manipulator,
	}
	manipulator.PathParse(path, result)

	return result
}

// Responsible for serialization of linux paths
type LinuxPathManipulator struct{ GenericPathManipulator }

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

func NewLinuxOSPath(path string) *OSPath {
	manipulator := LinuxPathManipulator{}
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

type WindowsPathManipulator struct{ GenericPathManipulator }

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

func (self WindowsPathManipulator) PathJoin(path *OSPath) string {
	result := self.AsPathSpec(path)
	if result.GetDelegateAccessor() == "" && result.GetDelegatePath() == "" {
		return result.Path
	}
	return result.String()
}

func (self WindowsPathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	}

	// The first component is usually the drive letter or device and
	// although it can contain path separators it must not be quoted
	components := path.Components

	if len(components) > 0 {
		// No leading \\ as first component is drive letter
		result.Path = components[0] + utils.JoinComponents(components[1:], "\\")
	}
	return result
}

func NewWindowsOSPath(path string) *OSPath {
	manipulator := WindowsPathManipulator{}
	result := &OSPath{
		Manipulator: manipulator,
	}
	manipulator.PathParse(path, result)

	return result
}

// Handle device paths especially.
type WindowsNTFSManipulator struct{ WindowsPathManipulator }

func (self WindowsNTFSManipulator) PathParse(path string, result *OSPath) {
	self.WindowsPathManipulator.PathParse(path, result)

	// Drive names are stored as devices in the ntfs accessors.  So if
	// a user specifies open C:\Windows, we automatically open the
	// \\.\C: device
	if len(result.Components) > 0 &&
		driveRegex.MatchString(result.Components[0]) {
		result.Components[0] = "\\\\.\\" + result.Components[0]
	}
}

func ConvertToDevice(component string) string {
	if driveRegex.MatchString(component) {
		return "\\\\.\\" + component
	}
	return component
}

func (self WindowsNTFSManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	}

	// The first component is usually the drive letter or device and
	// although it can contain path separators it must not be quoted
	components := path.Components

	switch len(components) {
	case 0:
		return result

	case 1:
		result.Path = components[0]

	default:
		// No leading \\ as first component is drive letter
		result.Path = components[0] + utils.JoinComponents(components[1:], "\\")
	}
	return result
}

func (self WindowsNTFSManipulator) PathJoin(path *OSPath) string {
	result := self.AsPathSpec(path)
	if result.GetDelegateAccessor() == "" && result.GetDelegatePath() == "" {
		return result.Path
	}
	return result.String()
}

func NewWindowsNTFSPath(path string) *OSPath {
	manipulator := WindowsNTFSManipulator{}
	result := &OSPath{
		Manipulator: manipulator,
	}
	manipulator.PathParse(path, result)

	return result
}

// Windows registry paths begin with a hive name. There are a number
// of abbreviations for the hive names and we want to standardize.
type WindowsRegistryPathManipulator struct{ GenericPathManipulator }

func (self WindowsRegistryPathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	}

	// The first component is usually the drive letter or device and
	// although it can contain path separators it must not be quoted
	components := path.Components

	result.Path = strings.TrimPrefix(utils.JoinComponents(components, "\\"), "\\")
	return result
}

func (self WindowsRegistryPathManipulator) PathJoin(path *OSPath) string {
	result := self.AsPathSpec(path)
	if result.GetDelegateAccessor() == "" && result.GetDelegatePath() == "" {
		return result.Path
	}
	return result.String()
}

func (self WindowsRegistryPathManipulator) PathParse(path string, result *OSPath) {
	maybeParsePathSpec(path, result)
	result.Components = utils.SplitComponents(result.pathspec.Path)

	if len(result.Components) > 0 {
		// First component is always a hive name in upper case.
		hive_name := strings.ToUpper(result.Components[0])
		switch hive_name {
		case "HKCU":
			hive_name = "HKEY_CURRENT_USER"
		case "HKLM":
			hive_name = "HKEY_LOCAL_MACHINE"
		case "HKU":
			hive_name = "HKEY_USERS"
		}
		result.Components[0] = hive_name
	}
}

func NewWindowsRegistryPath(path string) *OSPath {
	manipulator := WindowsRegistryPathManipulator{}
	result := &OSPath{
		Manipulator: manipulator,
	}
	manipulator.PathParse(path, result)

	return result
}

// Raw pathspec paths expect the path to be a json encoded PathSpec
// object. They do not have any special interpretation of the Path
// parameter and so they do not break it up at all. These are used in
// very limited situations when we do not want to represent
// hierarchical data at all.
type PathSpecPathManipulator struct{ GenericPathManipulator }

func (self PathSpecPathManipulator) PathParse(path string, result *OSPath) {
	pathspec, err := PathSpecFromString(path)
	if err == nil {
		result.pathspec = pathspec
	} else {
		result.pathspec = &PathSpec{Path: path}
	}

	result.Components = []string{pathspec.Path}
}

func (self PathSpecPathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	}
	return result
}

func (self PathSpecPathManipulator) PathJoin(path *OSPath) string {
	if path.pathspec != nil {
		return path.pathspec.String()
	}

	return ""
}

func NewPathspecOSPath(path string) *OSPath {
	manipulator := PathSpecPathManipulator{}
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

	// This is a hack to support old URL based pathspecs.

	// TODO: deprecate them completely in future.
	if strings.Contains(path, "#") {
		parsed_url, err := url.Parse(path)
		if err == nil {
			// Support urls for backwards compatibility.
			result.pathspec = &PathSpec{
				DelegateAccessor: parsed_url.Scheme,
				DelegatePath:     parsed_url.Path,
				Path:             parsed_url.Fragment,
				url_based:        true,
			}
			return
		}
	}

	result.pathspec = &PathSpec{
		Path: path,
	}
}
