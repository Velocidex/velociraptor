package utils

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
)

// Returns the containing dict for a nested dict. This allows fetching
// a key using dot notation.
func _get(dict *ordereddict.Dict, key string) (*ordereddict.Dict, string) {
	components := strings.Split(key, ".")
	// Only a single component, return the dict.
	if len(components) == 1 {
		return dict, components[0]
	}

	// Iterate over all but the last component fetching the nested
	// dicts. If any of these are not present or not a dict,
	// return an empty containing dict.
	for _, member := range components[:len(components)-1] {
		result, pres := dict.Get(member)
		if !pres {
			// Maybe it is an array
			if reflect.TypeOf(dict).Kind() == reflect.Slice {
				a_value := reflect.ValueOf(dict)
				index, err := strconv.Atoi(member)
				if err == nil && index > 0 && index < a_value.Len() {
					result = a_value.Index(index).Interface()
				}

			} else {
				return ordereddict.NewDict(), ""
			}
		}

		nested, ok := result.(*ordereddict.Dict)
		if !ok {
			return ordereddict.NewDict(), ""
		}
		dict = nested
	}

	return dict, components[len(components)-1]
}

func GetString(dict *ordereddict.Dict, key string) string {
	subdict, last := _get(dict, key)
	res, _ := subdict.GetString(last)
	return res
}

func GetInt64(dict *ordereddict.Dict, key string) int64 {
	subdict, last := _get(dict, key)
	res, _ := subdict.GetInt64(last)
	return res
}

func GetAny(dict *ordereddict.Dict, key string) vfilter.Any {
	subdict, last := _get(dict, key)
	res, _ := subdict.Get(last)
	return res
}
