/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
/*
   Velociraptor paths are a list of components (strings). Paths may be
   serialized into a string by joining the components using a path
   separator which can be / or \

   A path component itself may also contain path separators like / or
   \. If it does, then Velociraptor uses a special escaping scheme to
   preserve the component as one unit. Therefore path serialization
   followed by de-serialization is preserved on roundtrip.

   For example the path:

   "HKEY_USERS", "S-1-5-21-546003962-2713609280-610790815-1003",
   "Software", "Microsoft", "Windows", "CurrentVersion", "Run",
   "c:\\windows\\system32\\mshta.exe",

   Will be serialized into:

   \HKEY_USERS\S-1-5-21-546003962-2713609280-610790815-1003\Software\Microsoft\Windows\CurrentVersion\Run\"c:\windows\system32\mshta.exe"

*/
package utils

import (
	"regexp"
	"strings"
)

// Split the path into components. Note that since registry keys and
// values may contain path separators in their name, we need to ensure
// such names are escaped using quotes. For example:
// HKEY_USERS\S-1-5-21-546003962-2713609280-610790815-1003\Software\Microsoft\Windows\CurrentVersion\Run\"c:\windows\system32\mshta.exe"
var drive_letter_regex = regexp.MustCompile(`^[a-zA-Z]:$`)

func consumeComponent(path string) (next_path string, component string) {
	if len(path) == 0 {
		return "", ""
	}
	length := len(path)

	// The first character indicated the type of component.
	switch path[0] {
	// An empty component.
	case '/', '\\':
		return path[1:], ""

		// A quoted component - scan to the next quote
		// allowing for quote escapes by double quoting.
	case '"':
		result := []byte{}
		for i := 1; i < length; i++ {
			switch path[i] {
			case '"':
				// End of string.
				if i >= length-1 {
					return "", string(result)
				}

				// Peek at next char
				next_char := path[i+1]
				switch next_char {
				case '"': // Double quoted quote - unescape it
					result = append(result, next_char)
					i += 1
					continue

					// Path separator after quote
					// - end of component.
				case '/', '\\':
					return path[i+1 : length], string(result)
				default:
					// Should never happen, "
					// followed by anything
					result = append(result, next_char)
					continue
				}

			default:
				result = append(result, path[i])
			}
		}

		// If we get here it is unterminated (e.g. '"foo<EOF>')
		return "", string(result)

	default:
		for i := 0; i < length; i++ {
			switch path[i] {
			case '/', '\\':
				return path[i:length], path[:i]
			}
		}
	}

	return "", path
}

func SplitComponents(path string) []string {
	var components []string
	var component string

	for path != "" {
		path, component = consumeComponent(path)
		if component != "" && component != "." && component != ".." {
			components = append(components, component)
		}
	}

	return components
}

func escapeComponent(component string) string {
	length := len(component)
	if length > 1024 {
		length = 1024
	}

	hasQuotes := false
	result := make([]byte, 0, length*2)
	for i := 0; i < len(component); i++ {
		result = append(result, component[i])

		switch component[i] {
		case '/', '\\':
			hasQuotes = true
		case '"':
			hasQuotes = true
			result = append(result, '"')
		}
	}

	if hasQuotes {
		result = append(result, '"')
		result = append([]byte{'"'}, result...)
	}

	return string(result)
}

// The opposite of SplitComponents above.
func JoinComponents(components []string, sep string) string {
	if len(components) == 0 {
		return ""
	}

	result := []string{}
	for idx, component := range components {
		// If the first component looks like a drive letter
		// then omit the leading /
		if idx == 0 && drive_letter_regex.FindString(components[0]) == "" {
			result = append(result, "")
		}
		if component != "" {
			result = append(result, escapeComponent(component))
		}
	}
	return strings.Join(result, sep)
}

func Base(path string) string {
	components := SplitComponents(path)
	if len(components) > 0 {
		return components[len(components)-1]
	}
	return ""
}
