package data

import (
	"io"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestAccessorData(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	accessor, err := accessors.GetAccessor("data", scope)
	assert.NoError(t, err)

	fd, err := accessor.Open("Hello world")
	assert.NoError(t, err)

	data, err := io.ReadAll(fd)
	assert.NoError(t, err)

	assert.Equal(t, "Hello world", string(data))
}

func TestAccessorScope(t *testing.T) {
	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set("Foobar", "Hello world"))

	accessor, err := accessors.GetAccessor("scope", scope)
	assert.NoError(t, err)

	fd, err := accessor.Open("Foobar")
	assert.NoError(t, err)

	data, err := io.ReadAll(fd)
	assert.NoError(t, err)

	assert.Equal(t, "Hello world", string(data))
}
