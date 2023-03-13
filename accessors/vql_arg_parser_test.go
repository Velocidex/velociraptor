package accessors_test

import (
	"context"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

type testStruct struct {
	Path     *accessors.OSPath `vfilter:"required,field=path"`
	Accessor string            `vfilter:"optional,field=accessor"`
}

type testCases struct {
	name string
	in   vfilter.Any
}

var testcases = []testCases{
	{name: "Simple Path",
		in: []string{
			"Hello", "World",
		}},
	{name: "Path With {",
		in: []string{
			"Hello", "{this is a test}",
		}},
	{name: "FSPathSpec",
		in: path_specs.NewUnsafeFilestorePath("Hello", "World")},
	{name: "FSPathSpec With type",
		in: path_specs.NewUnsafeFilestorePath("Hello", "World").
			SetType(api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP)},
	{name: "DSPathSpec",
		in: path_specs.NewUnsafeDatastorePath("Hello", "World")},

	{name: "DSPathSpec With Type",
		in: path_specs.NewUnsafeDatastorePath("Hello", "World").
			SetType(api.PATH_TYPE_DATASTORE_PROTO)},

	{name: "OSPath",
		in: accessors.MustNewGenericOSPath("/foo/bar")},

	{name: "PathSpec",
		in: accessors.MustNewGenericOSPath("/foo/bar").PathSpec()},

	{name: "Serialized PathSpec",
		in: `{"Path": "/foo/bar.txt", "Accessor": "zip", "DelegatePath": "/tmp/file.zip", "DelegateAccessor": "file"}`},

	{name: "Multiple parts of mixed type",
		in: []vfilter.Any{accessors.MustNewGenericOSPath("/foo/bar"), "Hello.txt"}},

	// Just join all parts
	{name: "Multiple parts of mixed type",
		in: []vfilter.Any{"/root/home", accessors.MustNewGenericOSPath("/foo/bar"), "Hello.txt"}},

	{name: "Multiple parts of mixed type 2",
		in: []vfilter.Any{"/root/home", `{"Path": "/a/b"}`, "Hello.txt"}},
}

func TestVQLParsing(t *testing.T) {
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	result := ordereddict.NewDict()

	for _, testcase := range testcases {
		args := ordereddict.NewDict().
			Set("accessor", "file").
			Set("path", testcase.in)
		arg := &testStruct{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		assert.NoError(t, err)

		result.Set(testcase.name, ordereddict.NewDict().
			Set("Components", arg.Path.Components).
			Set("PathSpec", arg.Path.PathSpec()))
	}
	goldie.Assert(t, "TestVQLParsing", json.MustMarshalIndent(result))
}
