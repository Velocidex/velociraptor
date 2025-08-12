package acls

import (
	"reflect"
	"sort"
	"strings"

	"github.com/Velocidex/ordereddict"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func ParsePolicyFromDict(scope vfilter.Scope, in *ordereddict.Dict) (
	result *acl_proto.ApiClientACL, err error) {

	policy_map := in.ToMap()

	result = &acl_proto.ApiClientACL{}
	result_value := reflect.Indirect(reflect.ValueOf(result))

	// Get a list of fields
	var valid_fields []string
	res_type := result_value.Type()
	for i := 0; i < res_type.NumField(); i++ {
		field := res_type.Field(i)
		if !field.IsExported() {
			continue
		}

		field_name := strings.Split(field.Tag.Get("json"), ",")[0]
		switch field_name {
		case "roles":
			result.Roles, _ = in.GetStrings(field_name)
			valid_fields = append(valid_fields, field_name)

		case "publish_queues":
			result.PublishQueues, _ = in.GetStrings(field_name)
			valid_fields = append(valid_fields, field_name)

		case "super_user":

		default:
			valid_fields = append(valid_fields, field_name)
			value, pres := in.Get(field_name)
			if !pres {
				continue
			}

			// Field name is not the same as json name
			field_value := result_value.FieldByName(field.Name)
			if field.Type.Kind() != reflect.Bool || !field_value.CanSet() {
				continue
			}

			field_value.SetBool(scope.Bool(value))
		}

		delete(policy_map, field_name)
	}

	if len(policy_map) != 0 {
		var fields []string
		for k := range policy_map {
			fields = append(fields, k)
		}
		sort.Strings(fields)
		sort.Strings(valid_fields)
		return nil, utils.Wrap(utils.InvalidArgError, "Parsing Policy: Invalid policy fields: %v. Valid fields are %v", fields, valid_fields)
	}

	return result, nil
}
