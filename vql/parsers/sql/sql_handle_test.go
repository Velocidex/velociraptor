package sql

import (
	"testing"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

// Calling sql() repeatedly with the same mysql/postgres connstring
// in the same scope must reuse the cached handle instead of failing.
func TestGetHandleOtherReusesCachedHandle(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	plugin := SQLPlugin{}

	// sql.Open() is lazy so no server is needed.
	connstring := "postgres://user:pass@127.0.0.1:1/db?sslmode=disable"

	first, err := plugin.GetHandleOther(scope, connstring, "postgres")
	assert.NoError(t, err)

	second, err := plugin.GetHandleOther(scope, connstring, "postgres")
	assert.NoError(t, err)
	assert.True(t, first == second)
}
