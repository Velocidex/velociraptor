package tools

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/accessors/raw_registry"
	_ "www.velocidex.com/golang/velociraptor/vql/filesystem"
	_ "www.velocidex.com/golang/velociraptor/vql/protocols"
)

type RemapTestSuite struct {
	test_utils.TestSuite
}

func (self *RemapTestSuite) buildRemapConfig() *config_proto.Config {
	// Mount the raw registry hive on HKEY_LOCAL_MACHINE
	user_class_path, err := filepath.Abs("../../artifacts/testdata/files/UsrClass.dat")
	assert.NoError(self.T(), err)

	src_dir, err := filepath.Abs("../../artifacts/testdata/files/")
	assert.NoError(self.T(), err)

	// This maps the registry accessor from the raw hive
	// UserClass.dat. It will be mounted on
	// HKEY_CURRENT_USER\Software\Classes
	// We also map the test data directory on D: drive
	remap_config := fmt.Sprintf(`
remappings:
- type: mount
  from:
    accessor: "raw_reg"
    prefix: |
       {"DelegateAccessor":"file", "DelegatePath": %q}
  on:
    accessor: "registry"
    prefix: "HKEY_CURRENT_USER\\Software\\Classes"
    path_type: registry
- type: mount
  from:
    accessor: "file"
    prefix: %q
  on:
    prefix: "D:"
    path_type: windows
`, user_class_path, src_dir)

	config_obj := &config_proto.Config{}
	err = yaml.UnmarshalStrict([]byte(remap_config), config_obj)
	assert.NoError(self.T(), err)

	return config_obj
}

func (self *RemapTestSuite) TestConfigFileRemap() {
	config_obj := self.buildRemapConfig()
	self.ConfigObj.Remappings = config_obj.Remappings

	// Just build a standard scope.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	golden_fixture := ordereddict.NewDict()

	// Because the path_type is set to "registry", glob understands
	// HKCU shorthand.
	vql, err := vfilter.Parse(`
SELECT * FROM glob(globs='/HKCU/Software/Classes/windows*', accessor='registry')
ORDER BY FullPath
`)
	assert.NoError(self.T(), err)

	rows := []vfilter.Row{}
	for row := range vql.Eval(self.Ctx, scope) {
		rows = append(rows, row)
	}

	golden_fixture.Set("Registry Glob", rows)

	// Default accessor is the auto accessor.
	vql, err = vfilter.Parse(`
SELECT * FROM glob(globs='D:\\ntuser*')
ORDER BY FullPath
`)
	assert.NoError(self.T(), err)

	rows = []vfilter.Row{}
	for row := range vql.Eval(self.Ctx, scope) {
		rows = append(rows, row)
	}

	golden_fixture.Set("D drive Glob", rows)

	goldie.Assert(self.T(), "TestConfigFileRemap",
		json.MustMarshalIndent(golden_fixture))

}

// Check that we can apply mapping by using the remap() VQL function
func (self *RemapTestSuite) TestRemapByPlugin() {
	config_obj := self.buildRemapConfig()
	serialized, err := json.Marshal(config_obj)
	assert.NoError(self.T(), err)

	// Just build a standard scope.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict().
			Set("RemappingConfig", serialized),
	}

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	golden_fixture := ordereddict.NewDict()

	vql, err := vfilter.Parse(`
LET _ <= remap(config=RemappingConfig, clear=TRUE)
`)
	for _ = range vql.Eval(self.Ctx, scope) {
	}

	// Because the path_type is set to "registry", glob understands
	// HKCU shorthand.
	vql, err = vfilter.Parse(`
SELECT * FROM glob(globs='/HKCU/Software/Classes/windows*', accessor='registry')
ORDER BY FullPath
`)
	assert.NoError(self.T(), err)

	rows := []vfilter.Row{}
	for row := range vql.Eval(self.Ctx, scope) {
		rows = append(rows, row)
	}

	golden_fixture.Set("Registry Glob", rows)

	// Default accessor is the auto accessor.
	vql, err = vfilter.Parse(`
SELECT * FROM glob(globs='D:\\ntuser*')
ORDER BY FullPath
`)
	assert.NoError(self.T(), err)

	rows = []vfilter.Row{}
	for row := range vql.Eval(self.Ctx, scope) {
		rows = append(rows, row)
	}

	golden_fixture.Set("D drive Glob", rows)

	goldie.Assert(self.T(), "TestConfigFileRemap",
		json.MustMarshalIndent(golden_fixture))

}

func TestRemapPlugin(t *testing.T) {
	suite.Run(t, &RemapTestSuite{})
}
