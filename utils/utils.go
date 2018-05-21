package utils

import (
	"reflect"
)

func InString(hay *[]string, needle string) bool {
	for _, x := range *hay {
		if x == needle {
			return true
		}
	}

	return false
}

func IsNil(a interface{}) bool {
	defer func() { recover() }()
	return a == nil || reflect.ValueOf(a).IsNil()
}
