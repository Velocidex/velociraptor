package sparse

import (
	"io/ioutil"
	"testing"

	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
)

func TestAccessorSparse(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	accessor, err := accessors.GetAccessor("sparse", scope)
	assert.NoError(t, err)

	// The Path is really a json encoded sparse map.
	pathspec := &accessors.PathSpec{
		DelegateAccessor: "data",
		DelegatePath:     "This is a bit of text",
		Path:             `[{"length":5,"offset":0},{"length":3,"offset":10}]`,
	}

	fd, err := accessor.Open(pathspec.String())
	assert.NoError(t, err)

	data, err := ioutil.ReadAll(fd)
	assert.NoError(t, err)

	assert.Equal(t, "This \x00\x00\x00\x00\x00bit", string(data))
}
