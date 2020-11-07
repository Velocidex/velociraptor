package utils

import (
	"strings"

	"github.com/Velocidex/ordereddict"
)

// Returns the containing dict for a nested dict. This allows fetching
// a key using dot notation.
func _get(dict *ordereddict.Dict, key string) *ordereddict.Dict {
	components := strings.Split(key, ".")
	// Only a single component, return the dict.
	if len(components) == 1 {
		return dict
	}

	// Iterate over all but the last component fetching the nested
	// dicts. If any of these are not present or not a dict,
	// return an empty containing dict.
	for _, member := range components[:len(components)-1] {
		result, pres := dict.Get(member)
		if !pres {
			return ordereddict.NewDict()
		}
		nested, ok := result.(*ordereddict.Dict)
		if !ok {
			return ordereddict.NewDict()
		}
		dict = nested
	}

	return dict
}

func GetString(dict *ordereddict.Dict, key string) string {
	dict = _get(dict, key)
	res, _ := dict.GetString(key)
	return res
}

func GetInt64(dict *ordereddict.Dict, key string) int64 {
	dict = _get(dict, key)
	res, _ := dict.GetInt64(key)
	return res
}
