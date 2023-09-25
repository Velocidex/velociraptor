package utils

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// Velociraptor versioning historically was not strictly
// semantic. Here we try to massage the version into a valid semver so
// we can compare it properly. In future we should move Velociraptor
// to a more strict semver.
// v0.6.9 Means major 0 minor 6 and patch 9 as per standard -> 0.6.9
// v0.6.9-2 Means major 0, minor 69 and patch level 2 -> 0.69.2
// v0.6.9-rc2 Means major 0, minor 60, patch 9 and prerelease rc2 -> 0.6.9-rc2
//
// Therefore v0.6.9-rc2 < v0.6.9 < v0.6.9-2
// 0.60.9-rc2 < 0.6.9 < 0.69.2
// We never release an rc before a patch release.

var (
	velociraptor_rc_regex      = regexp.MustCompile(`v(\d)\.(\d)\.(\d)-rc(\d)`)
	velociraptor_post_regex    = regexp.MustCompile(`v(\d)\.(\d)\.(\d)-(\d)`)
	velociraptor_release_regex = regexp.MustCompile(`v(\d)\.(\d)\.(\d)`)
)

func normalizeVelciraptorVersion(v string) string {
	m := velociraptor_rc_regex.FindStringSubmatch(v)
	if len(m) > 0 {
		return fmt.Sprintf("v%v.%v.%v-rc%v", m[1], m[2], m[3], m[4])
	}

	m = velociraptor_post_regex.FindStringSubmatch(v)
	if len(m) > 0 {
		return fmt.Sprintf("v%v.%v%v.%v", m[1], m[2], m[3], m[4])
	}

	m = velociraptor_release_regex.FindStringSubmatch(v)
	if len(m) > 0 {
		return fmt.Sprintf("v%v.%v%v.0", m[1], m[2], m[3])
	}

	return v
}

func normalizeSemVer(version string) string {
	if !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}

func CompareVersions(tool, v, w string) int {
	v = normalizeSemVer(v)
	w = normalizeSemVer(w)
	if strings.Contains("velociraptor", strings.ToLower(tool)) {
		v = normalizeVelciraptorVersion(v)
		w = normalizeVelciraptorVersion(w)
	}
	return semver.Compare(v, w)
}
