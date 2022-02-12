package paths

import (
	"path/filepath"
	"regexp"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
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

// Breaks a client path into components. The client's path may consist
// of a drive letter or a device which will be treated as a single
// component. For example:
// C:\Windows -> "C:\", "Windows"
// \\.\c:\Windows -> "\\.\C:", "Windows"

// Other components that contain path separators need to be properly
// quoted as usual:
// HKEY_LOCAL_MACHINE\Software\Microsoft\"http://www.google.com"\Foo ->
// "HKEY_LOCAL_MACHINE", "Software", "Microsoft", "http://www.google.com", "Foo"
func ExtractClientPathSpec(accessor, path string) api.FSPathSpec {
	result := path_specs.NewUnsafeFilestorePath()
	if accessor != "" {
		result = result.AddChild(accessor)
	}

	components := ExtractClientPathComponents(path)

	// Restore the PathSpec type from its extensions
	if len(components) > 0 {
		last := len(components) - 1
		name_type, name := api.GetFileStorePathTypeFromExtension(
			components[last])
		components[last] = name
		result = result.SetType(name_type)
	}

	return result.AddChild(components...)
}

func ExtractClientPathComponents(path string) []string {
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

func clean(path string) string {
	result := filepath.Clean(path)
	if result == "." {
		result = ""
	}

	return result
}
