package actions

import (
	"github.com/stretchr/testify/assert"
	"testing"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
)

func TestCopy(t *testing.T) {
	name_1 := "1"
	name_2 := "2"
	name_3 := "3"
	a := &actions_proto.PathSpec{
		Path: &name_1,
		NestedPath: &actions_proto.PathSpec{
			Path: &name_2,
			NestedPath: &actions_proto.PathSpec{
				Path: &name_3,
			},
		},
	}
	assert.Equal(t, a, CopyPathspec(a))
}
