package utils

import (
	"reflect"
	"sort"
	"strings"
)

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

func InStringFolding(hay []string, needle string) bool {
	for _, x := range hay {
		if strings.EqualFold(x, needle) {
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

func SlicePrefixMatch(slice []string, prefix []string) bool {
	if len(slice) < len(prefix) {
		return false
	}

	return StringSliceEq(prefix, slice[:len(prefix)])
}

func FilterSlice(a []string, needle string) (res []string) {
	for _, i := range a {
		if i != needle {
			res = append(res, i)
		}
	}
	return res
}

func FilterSliceFolding(a []string, needle string) (res []string) {
	for _, i := range a {
		if !strings.EqualFold(i, needle) {
			res = append(res, i)
		}
	}
	return res
}

// Generic sorter. Useful to read maps in sorted order:
//
//	for _, k := range utils.Sort(mymap) {
//	  v := mymap[k]
//	  ...
//	}
func Sort(a interface{}) []string {
	var res []string

	a_value := reflect.Indirect(reflect.ValueOf(a))
	a_type := a_value.Type()

	if a_type.Kind() == reflect.Map {
		for _, k := range a_value.MapKeys() {
			res = append(res, k.String())
		}

	} else if a_type.Kind() == reflect.Slice {
		for i := 0; i < a_value.Len(); i++ {
			element := a_value.Index(i).Interface()
			element_str, ok := element.(string)
			if ok {
				res = append(res, element_str)
			}
		}
	}

	sort.Strings(res)

	return res
}
