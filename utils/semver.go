package utils

import (
	"strings"

	"golang.org/x/mod/semver"
)

func normalizeSemVer(version string) string {
	if !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}

func CompareVersions(v, w string) int {
	return semver.Compare(normalizeSemVer(v), normalizeSemVer(w))
}
