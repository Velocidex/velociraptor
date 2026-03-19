package acls

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func TestMergeACL(t *testing.T) {
	a := &acl_proto.ApiClientACL{
		Roles:         []string{"reader"},
		CollectServer: true,
	}

	b := &acl_proto.ApiClientACL{
		Roles:  []string{"org_admin"},
		Execve: true,
	}

	golden := ordereddict.NewDict()
	golden.Set("Merge", MergeACL(a, b))

	goldie.Assert(t, "TestMergeACL", json.MustMarshalIndent(golden))

}
