package accessors

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	osPathSerializations = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ospath_serialization_count",
		Help: "Number of times an os path is serialized.",
	})

	osPathUnserializations = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ospath_unserialization_count",
		Help: "Number of times an os path is unserialized.",
	})
)

// This is a generic Path manipulator that implements the escaping
// standard as used by Velociraptor:
//  1. Path separators are / but will be able to use \\ to parse.
//  2. Each component is optionally quoted if it contains special
//     characters (like path separators).
type GenericPathManipulator struct {
	Sep string
}

func (self GenericPathManipulator) ComponentEqual(a, b string) bool {
	return a == b
}

func (self GenericPathManipulator) PathParse(path string, result *OSPath) error {
	osPathUnserializations.Inc()

	err := maybeParsePathSpec(path, result)
	if err != nil {
		return err
	}
	result.Components = utils.SplitComponents(result.pathspec.Path)
	return nil
}

func (self GenericPathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	// Make a copy of the pathspec.
	var result PathSpec
	if path.pathspec != nil {
		result = *path.pathspec
	}

	components := path.Components
	sep := self.Sep
	if sep == "" {
		sep = "/"
	}
	result.Path = utils.JoinComponents(components, sep)
	return &result
}

func (self GenericPathManipulator) PathJoin(path *OSPath) string {
	osPathSerializations.Inc()

	result := self.AsPathSpec(path)
	if result.GetDelegateAccessor() == "" && result.GetDelegatePath() == "" {
		return result.Path
	}
	return result.String()
}

// Like NewGenericOSPath but panics if an error
func MustNewGenericOSPath(path string) *OSPath {
	res, err := NewGenericOSPath(path)
	if err != nil {
		panic(err)
	}
	return res
}

func MustNewGenericOSPathWithBackslashSeparator(path string) *OSPath {
	manipulator := GenericPathManipulator{Sep: "\\"}
	result := &OSPath{
		Manipulator: manipulator,
	}

	err := manipulator.PathParse(path, result)
	if err != nil {
		panic(err)
	}
	return result
}

func NewGenericOSPath(path string) (*OSPath, error) {
	manipulator := GenericPathManipulator{Sep: "/"}
	result := &OSPath{
		Manipulator: manipulator,
	}

	err := manipulator.PathParse(path, result)
	return result, err
}

// Responsible for serialization of linux paths
type LinuxPathManipulator struct{ GenericPathManipulator }

func (self LinuxPathManipulator) PathParse(path string, result *OSPath) error {
	osPathUnserializations.Inc()

	err := maybeParsePathSpec(path, result)
	if err != nil {
		return err
	}
	path = result.pathspec.Path

	components := strings.Split(path, "/")
	result.Components = make([]string, 0, len(components))
	for _, c := range components {
		if c == "" || c == "." || c == ".." {
			continue
		}
		result.Components = append(result.Components, c)
	}
	return nil
}

func (self LinuxPathManipulator) PathJoin(path *OSPath) string {
	osPathSerializations.Inc()

	result := self.AsPathSpec(path)
	if result.GetDelegateAccessor() == "" && result.GetDelegatePath() == "" {
		return result.Path
	}
	return result.String()
}

func (self LinuxPathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	} else {
		result = result.Copy()
	}
	result.Path = "/" + strings.Join(path.Components, "/")
	return result
}

func MustNewLinuxOSPath(path string) *OSPath {
	res, err := NewLinuxOSPath(path)
	if err != nil {
		panic(err)
	}
	return res
}

func NewLinuxOSPath(path string) (*OSPath, error) {
	manipulator := LinuxPathManipulator{}
	result := &OSPath{
		pathspec:    &PathSpec{},
		Manipulator: manipulator,
	}

	err := manipulator.PathParse(path, result)
	return result, err
}

var (
	// For convenience we transform paths like c:\Windows -> \\.\c:\Windows
	driveRegex = regexp.MustCompile(
		`(?i)^[/\\]?([a-z]:)(.*)`)

	// https://docs.microsoft.com/en-us/dotnet/standard/io/file-path-formats#unc-paths
	uncRegex = regexp.MustCompile(
		`(?i)^(\\\\[^\\]+)\\(.*)`)

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

// We also support UNC paths like:
// \\hostname\path\to\file -> "\\hostname", "path", "to", "file"

// Other components that contain path separators need to be properly
// quoted as usual:
// HKEY_LOCAL_MACHINE\Software\Microsoft\"http://www.google.com"\Foo ->
// "HKEY_LOCAL_MACHINE", "Software", "Microsoft", "http://www.google.com", "Foo"

type WindowsPathManipulator struct{ GenericPathManipulator }

func (self WindowsPathManipulator) ComponentEqual(a, b string) bool {
	return strings.EqualFold(a, b)
}

func (self WindowsPathManipulator) PathParse(path string, result *OSPath) error {
	osPathUnserializations.Inc()

	err := maybeParsePathSpec(path, result)
	if err != nil {
		return err
	}
	path = result.pathspec.Path

	m := deviceDriveRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		result.Components = append([]string{m[1]}, utils.SplitComponents(m[2])...)
		return nil
	}

	m = deviceDirectoryRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		result.Components = append([]string{m[1]}, utils.SplitComponents(m[2])...)
		return nil
	}

	m = uncRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		result.Components = append([]string{m[1]}, utils.SplitComponents(m[2])...)
		return nil
	}

	result.Components = utils.SplitComponents(path)
	return nil
}

func (self WindowsPathManipulator) PathJoin(path *OSPath) string {
	osPathSerializations.Inc()

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
	} else {
		result.Path = ""
	}
	return result
}

func MustNewWindowsOSPath(path string) *OSPath {
	res, err := NewWindowsOSPath(path)
	if err != nil {
		panic(err)
	}
	return res
}

func NewWindowsOSPath(path string) (*OSPath, error) {
	manipulator := WindowsPathManipulator{}
	result := &OSPath{
		Manipulator: manipulator,
	}
	err := manipulator.PathParse(path, result)
	return result, err
}

// Handle device paths especially.
type WindowsNTFSManipulator struct{ WindowsPathManipulator }

func (self WindowsNTFSManipulator) PathParse(path string, result *OSPath) error {
	err := self.WindowsPathManipulator.PathParse(path, result)
	if err != nil {
		return err
	}

	// Drive names are stored as devices in the ntfs accessors.  So if
	// a user specifies open C:\Windows, we automatically open the
	// \\.\C: device
	if len(result.Components) > 0 &&
		driveRegex.MatchString(result.Components[0]) {
		// Drive names should be uppercased
		result.Components[0] = "\\\\.\\" + strings.ToUpper(result.Components[0])
	}
	return nil
}

func ConvertToDevice(component string) string {
	if driveRegex.MatchString(component) {
		return "\\\\.\\" + strings.ToUpper(component)
	}
	return component
}

func (self WindowsNTFSManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	} else {
		result = result.Copy()
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
	osPathSerializations.Inc()

	result := self.AsPathSpec(path)
	if result.GetDelegateAccessor() == "" && result.GetDelegatePath() == "" {
		return result.Path
	}
	return result.String()
}

func MustNewWindowsNTFSPath(path string) *OSPath {
	res, err := NewWindowsNTFSPath(path)
	if err != nil {
		panic(err)
	}
	return res
}

func NewWindowsNTFSPath(path string) (*OSPath, error) {
	manipulator := WindowsNTFSManipulator{}
	result := &OSPath{
		Manipulator: manipulator,
	}
	err := manipulator.PathParse(path, result)
	return result, err
}

func WindowsNTFSPathFromOSPath(path *OSPath) *OSPath {
	result := &OSPath{
		Manipulator: WindowsNTFSManipulator{},
		Components:  make([]string, 0, len(path.Components)),
	}

	for i, component := range path.Components {
		if i == 0 {
			result.Components = append(result.Components,
				ConvertToDevice(component))
		} else {
			result.Components = append(result.Components, component)
		}
	}

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
	osPathSerializations.Inc()

	result := self.AsPathSpec(path)
	if result.GetDelegateAccessor() == "" && result.GetDelegatePath() == "" {
		return result.Path
	}
	return result.String()
}

func (self WindowsRegistryPathManipulator) PathParse(
	path string, result *OSPath) error {
	osPathUnserializations.Inc()

	err := maybeParsePathSpec(path, result)
	if err != nil {
		return err
	}
	result.Components = utils.SplitComponents(result.pathspec.Path)

	if len(result.Components) > 0 {
		// First component is usually a hive name in upper case.
		hive_name := result.Components[0]
		hive_name_caps := strings.ToUpper(result.Components[0])
		switch hive_name_caps {
		case "HKCU":
			hive_name = "HKEY_CURRENT_USER"
		case "HKLM":
			hive_name = "HKEY_LOCAL_MACHINE"
		case "HKU":
			hive_name = "HKEY_USERS"
		default:
			if strings.HasPrefix(hive_name, "HKEY_") {
				hive_name = hive_name_caps
			}
		}

		result.Components[0] = hive_name
	}
	return nil
}

func MustNewWindowsRegistryPath(path string) *OSPath {
	res, err := NewWindowsRegistryPath(path)
	if err != nil {
		panic(err)
	}
	return res
}

func NewWindowsRegistryPath(path string) (*OSPath, error) {
	manipulator := WindowsRegistryPathManipulator{}
	result := &OSPath{
		Manipulator: manipulator,
	}
	err := manipulator.PathParse(path, result)
	return result, err
}

// Raw pathspec paths expect the path to be a json encoded PathSpec
// object. They do not have any special interpretation of the Path
// parameter and so they do not break it up at all. These are used in
// very limited situations when we do not want to represent
// hierarchical data at all.
type PathSpecPathManipulator struct{ GenericPathManipulator }

func (self PathSpecPathManipulator) PathParse(path string, result *OSPath) error {
	osPathUnserializations.Inc()

	pathspec, err := PathSpecFromString(path)
	if err != nil {
		return err
	}
	result.pathspec = pathspec
	result.Components = []string{pathspec.Path}
	return nil
}

func (self PathSpecPathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	} else {
		result = result.Copy()
	}
	return result
}

func (self PathSpecPathManipulator) PathJoin(path *OSPath) string {
	osPathSerializations.Inc()

	if path.pathspec != nil {
		return path.pathspec.String()
	}

	if len(path.Components) == 1 {
		return path.Components[0]
	}

	return ""
}

func MustNewPathspecOSPath(path string) *OSPath {
	res, err := NewPathspecOSPath(path)
	if err != nil {
		panic(err)
	}
	return res
}

func NewPathspecOSPath(path string) (*OSPath, error) {
	manipulator := PathSpecPathManipulator{}
	result := &OSPath{
		Manipulator: manipulator,
	}

	err := manipulator.PathParse(path, result)
	return result, err
}

func maybeParsePathSpec(path string, result *OSPath) error {
	if strings.HasPrefix(path, "{") {
		pathspec := &PathSpec{}
		err := json.Unmarshal([]byte(path), pathspec)
		if err != nil {
			return fmt.Errorf("While decoding pathspec: %w", err)
		}
		result.pathspec = pathspec
		return nil
	}

	result.pathspec = &PathSpec{
		Path: path,
	}
	return nil
}

// Windows registry paths begin with a hive name. There are a number
// of abbreviations for the hive names and we want to standardize.
type FileStorePathManipulator struct{}

func (self FileStorePathManipulator) ComponentEqual(a, b string) bool {
	return a == b
}

func (self FileStorePathManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	} else {
		result = result.Copy()
	}

	// The first component is usually the drive letter or device and
	// although it can contain path separators it must not be quoted
	components := path.Components

	result.Path = strings.TrimPrefix(utils.JoinComponents(components, "/"), "/")
	return result
}

func (self FileStorePathManipulator) PathJoin(path *OSPath) string {
	osPathSerializations.Inc()

	return path.pathspec.DelegatePath + utils.JoinComponents(path.Components, "/")
}

func (self FileStorePathManipulator) PathParse(
	path string, result *OSPath) error {
	osPathUnserializations.Inc()

	err := maybeParsePathSpec(path, result)
	if err != nil {
		return err
	}
	result.Components = utils.SplitComponents(result.pathspec.Path)
	if len(result.Components) > 0 {
		if result.Components[0] == "fs:" {
			result.Components = result.Components[1:]
			result.pathspec = &PathSpec{
				DelegateAccessor: "fs",
				DelegatePath:     "fs:",
			}
			return nil
		}
		if result.Components[0] == "ds:" {
			result.Components = result.Components[1:]
			result.pathspec = &PathSpec{
				DelegateAccessor: "fs",
				DelegatePath:     "ds:",
			}
			return nil
		}
	}

	result.pathspec = &PathSpec{
		DelegateAccessor: "fs",
		DelegatePath:     "fs:",
	}
	return nil
}

// Like NewGenericOSPath but panics if an error
func MustNewFileStorePath(path string) *OSPath {
	res, err := NewFileStorePath(path)
	if err != nil {
		panic(err)
	}
	return res
}

func NewFileStorePath(path string) (*OSPath, error) {
	manipulator := &FileStorePathManipulator{}
	result := &OSPath{
		Manipulator: manipulator,
	}

	err := manipulator.PathParse(path, result)
	return result, err
}

// The OSPath object for raw files is unchanged - We must pass exactly
// the same form as given to the underlying filesystem APIs. On
// Windows this is some kind of device description like
// \\?\GLOBALROOT\Device\Harddisk0\DR0 for example, but we never
// attempt to parse it - just forward to the API as is.
type RawFileManipulator struct{}

func (self RawFileManipulator) ComponentEqual(a, b string) bool {
	return a == b
}

func (self RawFileManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := &PathSpec{}
	if len(path.Components) == 0 {
		return result
	}

	result.Path = path.Components[0]
	return result
}

func (self RawFileManipulator) PathJoin(path *OSPath) string {
	if len(path.Components) == 0 {
		return ""
	}
	return path.Components[0]
}

func (self RawFileManipulator) PathParse(
	path string, result *OSPath) error {
	result.Components = []string{path}
	return nil
}

func NewRawFilePath(path string) (*OSPath, error) {
	manipulator := &RawFileManipulator{}
	return &OSPath{
		Components:  []string{path},
		Manipulator: manipulator,
	}, nil
}

// Represent files inside the zip file for the offline collector -
// Similar to LinuxPathManipulator except that extra escaping is used
// to avoid more characters.
type ZipFileManipulator struct{}

func (self ZipFileManipulator) ComponentEqual(a, b string) bool {
	return strings.EqualFold(a, b)
}

func (self ZipFileManipulator) AsPathSpec(path *OSPath) *PathSpec {
	result := path.pathspec
	if result == nil {
		result = &PathSpec{}
		path.pathspec = result
	}
	components := make([]string, 0, len(path.Components))
	for _, c := range path.Components {
		if c != "" {
			components = append(components, utils.SanitizeStringForZip(c))
		}
	}
	result.Path = "/" + strings.Join(components, "/")
	return result
}

func (self ZipFileManipulator) PathJoin(path *OSPath) string {
	osPathSerializations.Inc()

	result := self.AsPathSpec(path)
	if result.GetDelegateAccessor() == "" && result.GetDelegatePath() == "" {
		return result.Path
	}
	return result.String()
}

func (self ZipFileManipulator) PathParse(
	path string, result *OSPath) error {
	osPathUnserializations.Inc()

	err := maybeParsePathSpec(path, result)
	if err != nil {
		return err
	}
	path = result.pathspec.Path

	components := strings.Split(path, "/")
	result.Components = make([]string, 0, len(components))
	for _, c := range components {
		if c == "" || c == "." || c == ".." {
			continue
		}
		result.Components = append(result.Components,
			utils.UnsanitizeComponentForZip(c))
	}
	return nil
}

func NewZipFilePath(path string) (*OSPath, error) {
	manipulator := &ZipFileManipulator{}
	result := &OSPath{
		Manipulator: manipulator,
	}
	err := manipulator.PathParse(path, result)
	return result, err
}

func MustNewZipFilePath(path string) *OSPath {
	res, err := NewZipFilePath(path)
	if err != nil {
		panic(err)
	}
	return res
}

func NewNativePath(path string) (*OSPath, error) {
	if runtime.GOOS == "windows" {
		return NewLinuxOSPath(path)
	} else {
		return NewWindowsOSPath(path)
	}
}
