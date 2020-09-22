package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type splitTest struct {
	path       string
	components []string
}

var splitTestCases = []splitTest{
	{"Hello", []string{"Hello"}},
	{"/foo/bar", []string{"foo", "bar"}},
	{"\\foo\\bar", []string{"foo", "bar"}},
	{"foo/bar", []string{"foo", "bar"}},
	{"foo/\"bar \"\"with quote\"", []string{"foo", "bar \"with quote"}},
	{"foo/\"bar with / component\"", []string{"foo", "bar with / component"}},

	// Invalid paths but should be handled.
	{"\\foo\\\"bar", []string{"foo", "bar"}},
	{"/foo/////\"bar", []string{"foo", "bar"}},
	{"/foo/////b\"ar", []string{"foo", "b\"ar"}}, // Should have been quoted

	{"//\"file\"/\"C:\"", []string{"file", "C:"}},
	{"//\"fi\"\"le\"/\"C:\"", []string{"fi\"le", "C:"}},

	// A registry path with included separators.
	{"HKEY_USERS\\S-1-5-21-546003962-2713609280-610790815-1003\\Software\\Microsoft\\Windows\\CurrentVersion\\Run\\\"c:\\windows\\system32\\mshta.exe\"",
		[]string{
			"HKEY_USERS", "S-1-5-21-546003962-2713609280-610790815-1003",
			"Software", "Microsoft", "Windows", "CurrentVersion", "Run",
			"c:\\windows\\system32\\mshta.exe",
		}},
}

func TestPathSplit(t *testing.T) {
	for _, test_case := range splitTestCases {
		result := SplitComponents(test_case.path)
		assert.Equal(t, result, test_case.components)
	}
}

type joinTest struct {
	path       string
	components []string
	sep        string
}

var joinTestCases = []joinTest{
	{"/Hello", []string{"Hello"}, "/"},
	{"/foo/bar", []string{"foo", "bar"}, "/"},
	{"/foo/\"bar \"\"with quote\"", []string{"foo", "bar \"with quote"}, "/"},
	{"/foo/\"bar with / component\"", []string{"foo", "bar with / component"}, "/"},

	// A registry path with included separators.
	{"\\HKEY_USERS\\S-1-5-21-546003962-2713609280-610790815-1003\\Software\\Microsoft\\Windows\\CurrentVersion\\Run\\\"c:\\windows\\system32\\mshta.exe\"",
		[]string{
			"HKEY_USERS", "S-1-5-21-546003962-2713609280-610790815-1003",
			"Software", "Microsoft", "Windows", "CurrentVersion", "Run",
			"c:\\windows\\system32\\mshta.exe",
		}, "\\"},

	// Components with a drive letter do not carry leading /
	{"C:/foo/bar", []string{"C:", "foo", "bar"}, "/"},

	// Devices are saved as complete components.
	{"/\"\\\\.\\C:\"/foo/bar", []string{"\\\\.\\C:", "foo", "bar"}, "/"},
}

func TestPathJoin(t *testing.T) {
	for _, test_case := range joinTestCases {
		result := JoinComponents(test_case.components, test_case.sep)
		assert.Equal(t, result, test_case.path)
	}
}
