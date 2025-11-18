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

var generic_testcases = []testcase{
	// Generic paths try to take a good guess of the path type:
	// 1. Use / or \ as path separator
	// 2. Quotes represent unbroken paths.
	{"/bin/file\\1.txt", []string{"bin", "file", "1.txt"}, "/bin/file/1.txt"},

	// Quotes in the filename are escaped by doubling up and enclosing
	// the component with a single quote.
	{"/bin/file\"1\".txt", []string{"bin", "file\"1\".txt"},
		`/bin/"file""1"".txt"`},

	{`/bin/"file""1"".txt"`, []string{"bin", "file\"1\".txt"},
		`/bin/"file""1"".txt"`},

	// Enclosing a path in quotes treats it as a single literal
	// component.
	{"/bin/\"file\\1.txt\"", []string{"bin", "file\\1.txt"}, "/bin/\"file\\1.txt\""},
}

func TestGenericManipulators(t *testing.T) {
	for _, testcase := range generic_testcases {
		path, err := NewGenericOSPath(testcase.serialized_path)
		assert.NoError(t, err)
		assert.Equal(t, testcase.components, path.Components)
		assert.Equal(t, testcase.expected_path, path.String())
	}
}

var linux_testcases = []testcase{
	{"/bin/ls", []string{"bin", "ls"}, "/bin/ls"},
	{"bin////ls", []string{"bin", "ls"}, "/bin/ls"},
	{"/bin/ls////", []string{"bin", "ls"}, "/bin/ls"},

	// Files with non-path backslash characters should be parsed as
	// one filename. They should also be serialized as a single file.
	{"/bin/file\\1.txt", []string{"bin", "file\\1.txt"}, "/bin/file\\1.txt"},

	// Ignore and dont support directory traversal at all
	{"/bin/../../../.././../../ls", []string{"bin", "ls"}, "/bin/ls"},

	// Can accept paths in pathspec format
	{"{\"Path\":\"/bin/ls\"}", []string{"bin", "ls"}, "/bin/ls"},
}

func TestLinuxManipulators(t *testing.T) {
	for _, testcase := range linux_testcases {
		path, err := NewLinuxOSPath(testcase.serialized_path)
		assert.NoError(t, err)
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

	// The drive letter must have a trailing \ otherwise the API uses
	// the current directory (e.g. dir C: vs dir C:\ )
	{"C:", []string{"C:"}, "C:"},

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
		path, err := NewWindowsOSPath(testcase.serialized_path)
		assert.NoError(t, err)
		assert.Equal(t, testcase.components, path.Components)
		assert.Equal(t, testcase.expected_path, path.String())
	}
}

var ntfs_testcases = []testcase{
	// Devices can contain \\ but it is preserved
	{"\\\\.\\C:\\Windows\\System32",
		[]string{"\\\\.\\C:", "Windows", "System32"},
		"\\\\.\\C:\\Windows\\System32"},

	// Devices should not have final \\ - the API requires to open
	// them without a trailing \
	{"\\\\.\\C:", []string{"\\\\.\\C:"}, "\\\\.\\C:"},

	// Handle VSS paths
	{"\\\\?\\GLOBALROOT\\Device\\HarddiskVolumeShadowCopy1\\Windows",
		[]string{"\\\\?\\GLOBALROOT\\Device\\HarddiskVolumeShadowCopy1", "Windows"},
		"\\\\?\\GLOBALROOT\\Device\\HarddiskVolumeShadowCopy1\\Windows"},
}

func TestWindowsNTFSManipulators(t *testing.T) {
	for _, testcase := range ntfs_testcases {
		path, err := NewWindowsNTFSPath(testcase.serialized_path)
		assert.NoError(t, err)
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
		path, err := NewWindowsRegistryPath(testcase.serialized_path)
		assert.NoError(t, err)
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
		path, err := NewPathspecOSPath(testcase.serialized_path)
		assert.NoError(t, err)
		assert.Equal(t, testcase.components, path.Components)
		assert.Equal(t, testcase.expected_path, path.String())
	}
}

// Raw Pathspec OSPath do not interpret the Path parameter in a
// special way - it is just being preserved. This is only used for
// accessors that use it to represent non-hierarchical data.
var filestore_testcases = []testcase{
	{"/clients/", []string{"clients"}, "fs:/clients"},
	{"ds:/clients/", []string{"clients"}, "ds:/clients"},
	{"fs:/clients/", []string{"clients"}, "fs:/clients"},
}

func TestFileStoreManipulators(t *testing.T) {
	for _, testcase := range filestore_testcases {
		path, err := NewFileStorePath(testcase.serialized_path)
		assert.NoError(t, err)
		assert.Equal(t, testcase.components, path.Components)
		assert.Equal(t, testcase.expected_path, path.String())
	}
}

// The ZipFileManipulator is used by the offline collector to abstract
// access to the collector zip files..
var zipfile_testcases = []testcase{
	{
		serialized_path: "{\"DelegateAccessor\":\"file\",\"DelegatePath\":\"/F.D4FD20VLKDJ2G.zip\",\"Path\":\"/uploads/auto/C%3A\"}",
		components:      []string{"uploads", "auto", "C:"},
	},
	{
		serialized_path: "{\"DelegateAccessor\":\"file\",\"DelegatePath\":\"/F.D4FD20VLKDJ2G.zip\",\"Path\":\"/uploads/ntfs/%5C%5C.%5CC%3A\"}",
		components:      []string{"uploads", "ntfs", `\\.\C:`},
	},
}

func TestZipFileManipulators(t *testing.T) {
	for _, testcase := range zipfile_testcases {
		path, err := NewZipFilePath(testcase.serialized_path)
		assert.NoError(t, err)
		assert.Equal(t, testcase.components, path.Components)
		expected_path := testcase.expected_path
		if expected_path == "" {
			expected_path = testcase.serialized_path
		}
		assert.Equal(t, expected_path, path.String())
	}
}
