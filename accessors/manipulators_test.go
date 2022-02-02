package accessors

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type testcase struct {
	serialized_path string
	components      []string
	expected_path   string
}

var linux_testcases = []testcase{
	{"/bin/ls", []string{"bin", "ls"}, "/bin/ls"},
	{"bin////ls", []string{"bin", "ls"}, "/bin/ls"},
	{"/bin/ls////", []string{"bin", "ls"}, "/bin/ls"},

	// Ignore and dont support directory traversal at all
	{"/bin/../../../.././../../ls", []string{"bin", "ls"}, "/bin/ls"},

	// Can accept paths in pathspec format
	{"{\"Path\":\"/bin/ls\"}", []string{"bin", "ls"}, "/bin/ls"},
}

func TestLinuxManipulators(t *testing.T) {
	for _, testcase := range linux_testcases {
		path := NewLinuxOSPath(testcase.serialized_path)
		assert.Equal(t, testcase.components, path.Components)
		assert.Equal(t, testcase.expected_path, path.String())
	}
}

var windows_testcases = []testcase{
	{"C:\\Windows\\System32",
		[]string{"C:", "Windows", "System32"},
		"C:\\Windows\\System32"},

	// We also support / as well but always serialized to \\
	{"C:/Windows/System32",
		[]string{"C:", "Windows", "System32"},
		"C:\\Windows\\System32"},

	// Devices can contain \\ but it is preserved
	{"\\\\.\\C:\\Windows\\System32",
		[]string{"\\\\.\\C:", "Windows", "System32"},
		"\\\\.\\C:\\Windows\\System32"},

	// Ignore and dont support directory traversal at all
	{"C:\\Windows\\System32\\..\\..\\..\\..\\ls",
		[]string{"C:", "Windows", "System32", "ls"},
		"C:\\Windows\\System32\\ls"},

	// Can accept paths in pathspec format
	{`{"Path":"C:\\Windows\\System32"}`, []string{
		"C:", "Windows", "System32"}, "C:\\Windows\\System32"},
}

func TestWindowsManipulators(t *testing.T) {
	for _, testcase := range windows_testcases {
		path := NewWindowsOSPath(testcase.serialized_path)
		assert.Equal(t, testcase.components, path.Components)
		assert.Equal(t, testcase.expected_path, path.String())
	}
}

var registry_testcases = []testcase{
	// Registry keys can contain slashes
	{"HKEY_LOCAL_MACHINE\\\"http://www.google.com\"\\Foo",
		[]string{"HKEY_LOCAL_MACHINE", "http://www.google.com", "Foo"},
		"HKEY_LOCAL_MACHINE\\\"http://www.google.com\"\\Foo"},

	// Registry keys can use shortcuts
	{"HKLM\\\"http://www.google.com\"\\Foo",
		[]string{"HKEY_LOCAL_MACHINE", "http://www.google.com", "Foo"},
		"HKEY_LOCAL_MACHINE\\\"http://www.google.com\"\\Foo"},
}

func TestRegistryManipulators(t *testing.T) {
	for _, testcase := range registry_testcases {
		path := NewWindowsRegistryPath(testcase.serialized_path)
		assert.Equal(t, testcase.components, path.Components)
		assert.Equal(t, testcase.expected_path, path.String())
	}
}

// Raw Pathspec OSPath do not interpret the Path parameter in a
// special way - it is just being preserved. This is only used for
// accessors that use it to represent non-hierarchical data.
var pathspec_testcases = []testcase{
	{"{\"DelegateAccessor\":\"zip\",\"DelegatePath\":\"Foo\",\"Path\":\"/bin/ls\"}",
		[]string{"/bin/ls"},
		"{\"DelegateAccessor\":\"zip\",\"DelegatePath\":\"Foo\",\"Path\":\"/bin/ls\"}"},
}

func TestPathspecManipulators(t *testing.T) {
	for _, testcase := range pathspec_testcases {
		path := NewPathspecOSPath(testcase.serialized_path)
		assert.Equal(t, testcase.components, path.Components)
		assert.Equal(t, testcase.expected_path, path.String())
	}
}
