package paths

import (
	"regexp"

	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// For convenience we transform paths like c:\Windows -> \\.\c:\Windows
	deviceDriveRegex = regexp.MustCompile(
		`(?i)^(\\\\[\?\.]\\[a-zA-Z]:)(.*)`)

	deviceDirectoryRegex = regexp.MustCompile(
		`(?i)^(\\\\[\?\.]\\GLOBALROOT\\Device\\[^/\\]+)([/\\]?.*)`)
)

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
