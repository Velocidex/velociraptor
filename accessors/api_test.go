package accessors

import (
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/json"
)

type api_tests struct {
	name       string
	path       string
	components string
}

var (
	// Trim the prefix from a path
	trim_tests = []api_tests{
		{"Simple Path",
			"C:/Windows/System32", "C:"},

		{"Simple Path Deep",
			"C:/Windows/System32", "C:,Windows"},

		{"Complex Pathspec",
			`{
 "DelegateAccessor": "raw_ntfs",
  "Delegate":{
    "DelegateAccessor": "file",
    "DelegatePath":"/mnt/flat",
    "Path":"/Windows/System32/Config/SYSTEM"
   },
   "Path":"ControlSet001"
}`,
			"ControlSet001"},

		// Trim just one prefix directory.
		{"Complex Pathspec Deep",
			`{
 "DelegateAccessor": "raw_ntfs",
  "Delegate":{
    "DelegateAccessor": "file",
    "DelegatePath":"/mnt/flat",
    "Path":"/Windows/System32/Config/SYSTEM"
   },
   "Path":"ControlSet001/Foo/Bar"
}`,
			"ControlSet001"},
	}

	append_tests = []api_tests{
		{"Simple Path",
			"C:/Windows/", "System32,notepad.exe"},

		{"Complex Pathspec",
			`{
 "DelegateAccessor": "raw_ntfs",
  "Delegate":{
    "DelegateAccessor": "file",
    "DelegatePath":"/mnt/flat",
    "Path":"/Windows/System32/Config/SYSTEM"
   },
   "Path":"ControlSet001"
}`,
			"Foo,Bar"},
	}
)

// Make sure OSPath can handle complex path manipulations
func TestOSPathOperationsTrimComponents(t *testing.T) {
	result := ordereddict.NewDict()
	for _, test_case := range trim_tests {
		a := MustNewWindowsOSPath(test_case.path)
		components := strings.Split(test_case.components, ",")
		trimmed := a.TrimComponents(components...)
		result.Set(test_case.name, trimmed)
	}

	goldie.Assert(t, "TestOSPathOperationsTrimComponents",
		json.MustMarshalIndent(result))
}

func TestOSPathOperationsAppendComponents(t *testing.T) {
	result := ordereddict.NewDict()
	for _, test_case := range append_tests {
		a := MustNewWindowsOSPath(test_case.path)
		components := strings.Split(test_case.components, ",")
		trimmed := a.Append(components...)
		result.Set(test_case.name, trimmed)
	}

	goldie.Assert(t, "TestOSPathOperationsAppendComponents",
		json.MustMarshalIndent(result))
}
