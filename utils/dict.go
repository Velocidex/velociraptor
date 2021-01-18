package utils

import (
	"strings"

	"github.com/Velocidex/ordereddict"
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
			return ordereddict.NewDict(), ""
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
