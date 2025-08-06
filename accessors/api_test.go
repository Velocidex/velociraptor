package accessors_test

import (
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/zip"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
	_ "www.velocidex.com/golang/velociraptor/accessors/offset"
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
		a := accessors.MustNewWindowsOSPath(test_case.path)
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
		a := accessors.MustNewWindowsOSPath(test_case.path)
		components := strings.Split(test_case.components, ",")
		trimmed := a.Append(components...)
		result.Set(test_case.name, trimmed)
	}

	goldie.Assert(t, "TestOSPathOperationsAppendComponents",
		json.MustMarshalIndent(result))
}

type human_string_tests_t struct {
	name      string
	pathspec  string
	path_type string
}

var human_string_tests = []human_string_tests_t{
	{"Deep Pathspec",
		`{
        "Path": "/ControlSet001",
        "DelegateAccessor": "raw_ntfs",
        "Delegate": {
          "DelegateAccessor":"offset",
          "Delegate": {
            "DelegateAccessor": "virt",
            "DelegatePath": "/shared/mnt/flat",
            "Path": "122683392"
          },
          "Path":"/Windows/System32/Config/SYSTEM"
        }
      }
`, "linux"},
	{"Normal path", `C:\Windows\System32`, "windows"},
}

func TestOSPathHumanString(t *testing.T) {
	config_obj := &config_proto.Config{}

	// To make this test run on Linux and Windows the same we use a
	// neutral accessor.
	device_manager := accessors.GetDefaultDeviceManager(config_obj).Copy()
	device_manager.Register(accessors.DescribeAccessor(
		accessors.NewVirtualFilesystemAccessor(accessors.MustNewLinuxOSPath("")),
		accessors.AccessorDescriptor{
			Name: "virt",
		}))

	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}).
		Set(constants.SCOPE_DEVICE_MANAGER, device_manager))

	result := ordereddict.NewDict()
	for _, test_case := range human_string_tests {
		switch test_case.path_type {
		case "linux":
			a := accessors.MustNewLinuxOSPath(test_case.pathspec)
			result.Set(test_case.name, a.HumanString(scope))

		case "windows":
			a := accessors.MustNewWindowsOSPath(test_case.pathspec)
			result.Set(test_case.name, a.HumanString(scope))
		}
	}
	goldie.Assert(t, "TestOSPathHumanString",
		json.MustMarshalIndent(result))
}

func init() {
	// Override the file accessor with something that uses Generic
	// ospath so tests are the same on windows and linux.
	accessors.Register(&zip.ZipFileSystemAccessor{})
}
