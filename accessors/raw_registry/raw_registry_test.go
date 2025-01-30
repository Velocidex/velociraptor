package raw_registry

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
)

func TestAccessorRawReg(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(logging.NewPlainLogger(
		config_obj, &logging.FrontendComponent))

	runtest := func(scope vfilter.Scope) ([]string, error) {
		reg_accessor, err := accessors.GetAccessor("raw_reg", scope)
		if err != nil {
			return nil, err
		}

		abs_path, _ := filepath.Abs("../../artifacts/testdata/files/SAM")
		root := &accessors.PathSpec{
			DelegateAccessor: "file",
			DelegatePath:     abs_path,
		}
		root_path, err := accessors.NewWindowsOSPath(root.String())
		assert.NoError(t, err)

		globber := glob.NewGlobber()
		defer globber.Close()

		glob_path, err := accessors.NewLinuxOSPath("/SAM/Domains/*/*")
		assert.NoError(t, err)

		globber.Add(glob_path)

		hits := []string{}
		for hit := range globber.ExpandWithContext(
			context.Background(), scope, config_obj, root_path, reg_accessor) {
			hits = append(hits, hit.OSPath().Path())
		}

		sort.Strings(hits)
		return hits, nil
	}

	// Check the logs - permission should be denied.
	logging.ClearMemoryLogs()

	_, err := runtest(scope)
	assert.NoError(t, err)

	assert.Contains(t, strings.Join(logging.GetMemoryLogs(), ""),
		"Permission denied: [FILESYSTEM_READ]")

	// Now repeat with proper access
	scope = vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))

	hits, err := runtest(scope)
	assert.NoError(t, err)

	goldie.Assert(t, "TestAccessorRawReg", json.MustMarshalIndent(hits))
}
