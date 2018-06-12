package actions

import (
	"github.com/stretchr/testify/assert"
	"testing"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
)

func TestCopy(t *testing.T) {
	a := &actions_proto.PathSpec{
		Path: "1",
		NestedPath: &actions_proto.PathSpec{
			Path: "2",
			NestedPath: &actions_proto.PathSpec{
				Path: "3",
			},
		},
	}
	assert.Equal(t, a, CopyPathspec(a))
}
