package acls

import (
	"reflect"
	"sort"

	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func ACLEqual(a, b *acl_proto.ApiClientACL) bool {
	sort.Strings(a.Roles)
	sort.Strings(b.Roles)

	return utils.StringSliceEq(a.Roles, b.Roles) && a == b
}

func CopyACL(old *acl_proto.ApiClientACL) *acl_proto.ApiClientACL {
	res := *old
	res.Roles = utils.CopySlice(old.Roles)
	return &res
}

// Merge the new ACL into the old
func MergeACL(old, new *acl_proto.ApiClientACL) *acl_proto.ApiClientACL {
	old = CopyACL(old)

	old.Roles = utils.DeduplicateStringSlice(append(old.Roles, new.Roles...))

	// Now set the individual ACLs
	old_value := reflect.Indirect(reflect.ValueOf(old))
	new_value := reflect.Indirect(reflect.ValueOf(new))

	res_type := old_value.Type()
	for i := 0; i < res_type.NumField(); i++ {
		field := res_type.Field(i)
		if !field.IsExported() {
			continue
		}

		if field.Type.Kind() != reflect.Bool {
			continue
		}

		old_value := old_value.FieldByName(field.Name)
		old_bool := old_value.Interface().(bool)
		if !old_bool {
			new_value := new_value.FieldByName(field.Name)
			new_bool := new_value.Interface().(bool)
			if new_bool {
				old_value.SetBool(true)
			}
		}
	}

	return old
}
