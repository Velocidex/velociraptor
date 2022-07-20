package zip

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
)

func TestAccessorGzip(t *testing.T) {
	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	gzip_accessor, err := accessors.GetAccessor("gzip", scope)
	assert.NoError(t, err)

	abs_path, _ := filepath.Abs("../../artifacts/testdata/files/hi.gz")

	fd, err := gzip_accessor.Open(abs_path)
	assert.NoError(t, err)

	data, err := ioutil.ReadAll(fd)
	assert.NoError(t, err)

	assert.Equal(t, "hello world\n", string(data))
}

func TestAccessorBzip2(t *testing.T) {
	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	gzip_accessor, err := accessors.GetAccessor("bzip2", scope)
	assert.NoError(t, err)

	abs_path, _ := filepath.Abs("../../artifacts/testdata/files/goodbye.bz2")

	fd, err := gzip_accessor.Open(abs_path)
	assert.NoError(t, err)

	data, err := ioutil.ReadAll(fd)
	assert.NoError(t, err)

	assert.Equal(t, "goodbye world\n", string(data))
}
