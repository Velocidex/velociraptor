/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
package utils

import (
	"path"
	"regexp"
	"strings"
)

// Split the path into components. Note that since registry keys and
// values may contain path separators in their name, we need to ensure
// such names are escaped using quotes. For example:
// HKEY_USERS\S-1-5-21-546003962-2713609280-610790815-1003\Software\Microsoft\Windows\CurrentVersion\Run\"c:\windows\system32\mshta.exe"
var component_quoted_regex = regexp.MustCompile(`^"((?:[^"\\]*(?:\\"?)?)+)"`)
var component_unquoted_regex = regexp.MustCompile(`^[\\/]?([^\\/]*)([\\/]?|$)`)
var drive_letter_regex = regexp.MustCompile(`^[a-zA-Z]:$`)

func SplitComponents(path string) []string {
	var components []string
	for len(path) > 0 {
		match := component_quoted_regex.FindStringSubmatch(path)
		if len(match) > 0 {
			if len(match[1]) > 0 {
				components = append(components, match[1])
			}
			path = path[len(match[0]):]
			continue
		}
		match = component_unquoted_regex.FindStringSubmatch(path)
		if len(match) > 0 {
			if len(match[1]) > 0 {
				components = append(components, match[1])
			}
			path = path[len(match[0]):]
			continue
		}

		// This should never happen!
		return strings.Split(path, "\\")
	}
	return components
}

// The opposite of SplitComponents above.
func JoinComponents(components []string, sep string) string {
	result := []string{}
	for _, component := range components {
		// The component contains any separator then we must
		// escape it.
		if strings.Contains(component, "\\") ||
			strings.Contains(component, "/") {
			component = "\"" + component + "\""
		}
		result = append(result, component)
	}

	if len(components) > 0 &&
		drive_letter_regex.FindString(components[0]) == "" {
		return sep + strings.Join(result, sep)
	}

	return strings.Join(result, sep)
}

func PathJoin(root, stem, sep string) string {
	// If any of the subsequent components contain
	// a slash then escape them together.
	if strings.Contains(stem, "/") || strings.Contains(stem, "\\") {
		stem = "\"" + stem + "\""
	}

	return root + sep + stem
}

// Figure out where to store the VFSDownloadInfo file.
func GetVFSDownloadInfoPath(client_id, accessor, client_path string) string {
	return path.Join(
		"clients", client_id,
		"vfs_files", accessor,
		Normalize_windows_path(client_path))
}

// GetVFSDownloadInfoPath returns the data store path to the directory
// info file.
func GetVFSDirectoryInfoPath(client_id, accessor, client_path string) string {
	return path.Join(
		"clients", client_id,
		"vfs", accessor,
		Normalize_windows_path(client_path))
}
