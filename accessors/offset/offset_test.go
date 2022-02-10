package offset

import (
	"io/ioutil"
	"os"
	"testing"

	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
)

func TestAccessorOffset(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	accessor, err := accessors.GetAccessor("offset", scope)
	assert.NoError(t, err)

	// The Path is really a json encoded sparse map.
	pathspec := &accessors.PathSpec{
		DelegateAccessor: "data",
		DelegatePath:     "This is a bit of text",
		Path:             `10`,
	}

	fd, err := accessor.Open(pathspec.String())
	assert.NoError(t, err)

	data, err := ioutil.ReadAll(fd)
	assert.NoError(t, err)

	assert.Equal(t, "bit of text", string(data))

	// Check that Seeking works
	n, err := fd.Seek(3, os.SEEK_SET)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), n)

	m, err := fd.Read(data)
	assert.NoError(t, err)
	assert.Equal(t, " of text", string(data[:m]))
}
