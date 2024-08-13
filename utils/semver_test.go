package utils

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type testCaseSemVer struct {
	tool          string
	before, after string
}

var testCases = []testCaseSemVer{
	{"Velociraptor", "v0.6.9-rc2", "v0.6.9"},
	{"Velociraptor", "v0.6.9", "v0.6.9-2"},

	// The rc1 of the next version is higher than the release of the
	// current version!
	{"Velociraptor", "v0.7.0-4", "v0.7.1-rc1"},
	{"Velociraptor", "v0.7.0-rc1", "v0.7.0-5"},
	{"Velociraptor", "v0.7.0-4", "v0.7.0-5"},
	{"Velociraptor", "v0", constants.VERSION},
	{"Velociraptor", constants.VERSION, "v1011.12.121"},
}

func TestSemver(t *testing.T) {
	for _, test_case := range testCases {
		assert.Equal(t, -1,
			CompareVersions(test_case.tool,
				test_case.before, test_case.after),
			"Failed match %v < %v", test_case.before, test_case.after)
	}
}
