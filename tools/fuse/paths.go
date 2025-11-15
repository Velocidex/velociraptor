package fuse

import (
	"regexp"

	"www.velocidex.com/golang/velociraptor/accessors"
)

var (
	deviceLetterRegex = regexp.MustCompile(`^\\\\.\\([A-Za-z]:)$`)
	driveLetterRegex  = regexp.MustCompile(`^([A-Za-z]):$`)

	windowsForbiddenChars = regexp.MustCompile(`[<>:"/\\|?*]`)
	linuxForbiddenChars   = regexp.MustCompile(`[/]`)
)

func (self *Options) RemapPath(path *accessors.OSPath) string {
	basename := path.Basename()

	// Map all the accessors into a files directory
	if self.MergeAllAccessors && len(path.Components) == 2 {
		return "files"
	}

	if self.MapDeviceNamesToLetters {
		matches := deviceLetterRegex.FindStringSubmatch(basename)
		if len(matches) > 1 {
			basename = matches[1]
		}
	}

	if self.MapDriveNamesToLetters {
		matches := driveLetterRegex.FindStringSubmatch(basename)
		if len(matches) > 1 {
			basename = matches[1]
		}
	}

	re := windowsForbiddenChars
	if self.UnixCompatiblePathEscaping {
		re = linuxForbiddenChars
	}

	basename = re.ReplaceAllStringFunc(basename,
		func(m string) string {
			runes := []rune(m)
			if len(runes) > 0 {
				return string(escapeChar(runes[0]))
			}
			return m
		})

	return basename
}

var hexTable = []byte("0123456789ABCDEF")

func escapeChar(c rune) []rune {
	return []rune{'%', rune(hexTable[c>>4]), rune(hexTable[c&15])}
}
