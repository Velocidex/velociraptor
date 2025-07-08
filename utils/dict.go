package utils

import (
	"encoding/json"
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
	for i := 0; i < len(components)-1; i++ {
		member := components[i]
		result, pres := dict.Get(member)
		if !pres {
			return ordereddict.NewDict(), ""
		}

		// Maybe it is an array. If it is check if the next component
		// is an index.
		if reflect.TypeOf(result).Kind() == reflect.Slice &&
			i < len(components)-1 {
			a_value := reflect.ValueOf(result)

			next_member := components[i+1]
			index, err := strconv.Atoi(next_member)
			if err == nil {
				// Index out of range
				if index < 0 || index > a_value.Len() {
					return ordereddict.NewDict(), ""
				}

				dict = ordereddict.NewDict().
					Set(next_member, a_value.Index(index).Interface())
				continue
			}
		}

		nested, ok := result.(*ordereddict.Dict)
		if !ok || nested == nil {
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
	res, pres := subdict.GetInt64(last)
	if !pres {
		res_str, pres := subdict.GetString(last)
		if pres {
			res, _ = strconv.ParseInt(res_str, 0, 64)
		}
	}
	return res
}

func GetAny(dict *ordereddict.Dict, key string) vfilter.Any {
	subdict, last := _get(dict, key)
	res, _ := subdict.Get(last)
	return res
}

func ToPureDict(a interface{}) (*ordereddict.Dict, error) {
	serialized, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}

	result := ordereddict.NewDict()
	err = json.Unmarshal(serialized, result)

	return result, err
}
