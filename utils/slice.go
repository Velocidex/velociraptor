package utils

import (
	"reflect"

	"github.com/Velocidex/ordereddict"
)

func DictGetStringSlice(a *ordereddict.Dict, field string) []string {
	value, ok := a.Get(field)
	if ok {
		return ConvertToStringSlice(value)
	}
	return nil
}

func ConvertToStringSlice(a interface{}) []string {
	result := []string{}

	a_value := reflect.Indirect(reflect.ValueOf(a))
	a_type := a_value.Type()

	if a_type.Kind() == reflect.Slice {
		for i := 0; i < a_value.Len(); i++ {
			element := a_value.Index(i).Interface()
			element_str, ok := element.(string)
			if ok {
				result = append(result, element_str)
			}
		}
	}
	return result
}

func DeduplicateStringSlice(in []string) (out []string) {
	for _, i := range in {
		if !InString(out, i) {
			out = append(out, i)
		}
	}
	return out
}

func InString(hay []string, needle string) bool {
	for _, x := range hay {
		if x == needle {
			return true
		}
	}

	return false
}

func StringSliceEq(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func FilterSlice(a []string) (res []string) {
	for _, i := range a {
		if i != "" {
			res = append(res, i)
		}
	}
	return res
}
