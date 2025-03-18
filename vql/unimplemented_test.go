package vql_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/common"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"
)

type TestSuite struct {
	test_utils.TestSuite
}

func (self *TestSuite) TestUnimplementedPlugins() {
	scope := vql_subsystem.MakeScope()
	vql_subsystem.InstallUnimplemented(scope)

	scope = vql_subsystem.MakeScope()

	scope.SetLogger(logging.NewPlainLogger(
		self.ConfigObj, &logging.FrontendComponent))
	defer scope.Close()

	plugin_name := "watch_etw"
	if runtime.GOOS == "windows" {
		plugin_name = "watch_ebpf"
	}

	query := fmt.Sprintf(`
SELECT * FROM %s()
`, plugin_name)

	vql, err := vfilter.Parse(query)
	assert.NoError(self.T(), err)

	rows := []vfilter.Row{}
	for row := range vql.Eval(self.Ctx, scope) {
		rows = append(rows, row)
	}

	vtesting.MemoryLogsContain(self.T(), "Plugin .+ is not implemented for this architecture")

	version_check := common.GetVersion{}.Call(
		self.Ctx, scope, ordereddict.NewDict().Set("plugin", plugin_name))
	assert.Equal(self.T(), version_check, vfilter.Null{})
}

func TestUnimplemented(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
