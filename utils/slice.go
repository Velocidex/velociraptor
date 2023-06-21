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
