package raw_registry

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

func TestAccessorRawReg(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	reg_accessor, err := accessors.GetAccessor("raw_reg", scope)
	assert.NoError(t, err)

	abs_path, _ := filepath.Abs("../../artifacts/testdata/files/SAM")
	root := &accessors.PathSpec{
		DelegateAccessor: "file",
		DelegatePath:     abs_path,
	}
	root_path := accessors.NewPathspecOSPath(root.String())

	globber := glob.NewGlobber()
	globber.Add(accessors.NewLinuxOSPath("/SAM/Domains/*/*"))

	hits := []string{}
	for hit := range globber.ExpandWithContext(
		context.Background(), config_obj, root_path, reg_accessor) {
		hits = append(hits, hit.OSPath().PathSpec().Path)
	}

	goldie.Assert(t, "TestAccessorRawReg", json.MustMarshalIndent(hits))
}
