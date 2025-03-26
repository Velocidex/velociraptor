/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package utils

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/json"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

func hard_wrap(text string, colBreak int) string {
	text = strings.TrimSpace(text)
	text = strings.Replace(text, "\r\n", "\n", -1)
	wrapped := ""
	var i int
	for i = 0; len(text[i:]) > colBreak; i += colBreak {

		wrapped += text[i:i+colBreak] + "\n"

	}
	wrapped += text[i:]

	return wrapped
}

func Stringify(value interface{}, scope vfilter.Scope, min_width int) string {
	// Deal with pointers to things as those things.
	if reflect.TypeOf(value).Kind() == reflect.Ptr {
		return Stringify(reflect.Indirect(
			reflect.ValueOf(value)).Interface(), scope, min_width)
	}

	if reflect.TypeOf(value).Kind() == reflect.Slice {
		result := []string{}
		a_value := reflect.ValueOf(value)

		for i := 0; i < a_value.Len(); i++ {
			result = append(
				result, Stringify(
					a_value.Index(i).Interface(), scope, min_width))
		}

		return strings.Join(result, "\n")
	}

	json_marshall := func(value interface{}) string {
		if k, err := vjson.Marshal(value); err == nil {
			if len(k) > 0 && k[0] == '"' && k[len(k)-1] == '"' {
				k = k[1 : len(k)-1]
			}

			return hard_wrap(string(k), min_width)
		}
		return ""
	}

	switch t := value.(type) {

	case ordereddict.Dict:
		return t.String()

	case map[string]interface{}:
		result := []string{}
		for k, v := range t {
			result = append(result, fmt.Sprintf("%v: %v", k, v))
		}
		return strings.Join(result, "\n")

	case types.StringProtocol:
		return t.ToString(scope)

	case []byte:
		return hard_wrap(string(t), min_width)

	case string:
		return hard_wrap(t, min_width)

	//  If we have a custom marshaller we use it.
	case json.Marshaler:
		return json_marshall(value)

	default:
		// For normal structs json is a pretty good encoder.
		if reflect.TypeOf(value).Kind() == reflect.Struct {
			return json_marshall(value)
		}

		// Anything else we output something useful.
		return hard_wrap(fmt.Sprintf("%v", value), min_width)
	}
}

func BytesEqual(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for idx, a_item := range a {
		if a_item != b[idx] {
			return false
		}
	}

	return true
}

// Force coersion to int64
func ToInt64(x interface{}) (int64, bool) {
	switch t := x.(type) {
	case bool:
		if t {
			return 1, true
		} else {
			return 0, true
		}
	case int:
		return int64(t), true
	case uint8:
		return int64(t), true
	case int8:
		return int64(t), true
	case uint16:
		return int64(t), true
	case int16:
		return int64(t), true
	case uint32:
		return int64(t), true
	case int32:
		return int64(t), true
	case uint64:
		return int64(t), true
	case int64:
		return t, true

	case string:
		value, err := strconv.ParseInt(t, 0, 64)
		return value, err == nil

	case float64:
		return int64(t), true

	default:
		return 0, false
	}
}

func ParseIntoProtobuf(source interface{}, destination proto.Message) error {
	if source == nil {
		return errors.New("Nil")
	}

	serialized, err := json.Marshal(source)
	if err != nil {
		return err
	}

	return protojson.Unmarshal(serialized, destination)
}
