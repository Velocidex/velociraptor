package json

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/Velocidex/json"
	"www.velocidex.com/golang/vfilter"
)

var (
	number_regex = regexp.MustCompile(
		`^(?i)(?P<Number>[-+]?\d*\.?\d+([eE][-+]?\d+)?)$`)

	// Strings that look like this will be escaped because they
	// might be confused with other things.
	protected_prefix = regexp.MustCompile(
		`(?i)^( |\{|\[|true|false|[+-]?inf|base64:)`)
)

func AnyToString(item vfilter.Any, opts *json.EncOpts) string {
	value := ""

	switch t := item.(type) {
	case float32:
		value = strconv.FormatFloat(float64(t), 'f', -1, 64)

	case float64:
		value = strconv.FormatFloat(t, 'f', -1, 64)

	case time.Time:
		// Use the encoding options to control how to serialize the
		// time into the correct timezone.
		serialized, err := MarshalIndentWithOptions(t, opts)
		if err != nil || len(serialized) < 10 {
			return ""
		}
		// Strip the quote marks so it is a bare string value.
		return string(serialized[1 : len(serialized)-1])

	case int, int16, int32, int64, uint16, uint32, uint64, bool:
		value = fmt.Sprintf("%v", item)

	case []byte:
		value = "base64:" + base64.StdEncoding.EncodeToString(t)

	case string:
		// If the string looks like a number we encode
		// it as a json object. This will ensure that
		// the reader does not get confused between
		// strings which look like a number and
		// numbers.
		if number_regex.MatchString(t) ||
			protected_prefix.MatchString(t) {
			value = " " + t
		} else {
			value = t
		}

	default:
		serialized, err := MarshalIndentWithOptions(item, opts)
		if err != nil {
			return ""
		}

		if len(serialized) > 0 {
			if serialized[0] == '{' || serialized[0] == '[' {
				value = string(serialized)
			} else if serialized[0] == '"' && serialized[len(serialized)-1] == '"' {
				value, _ = strconv.Unquote(string(serialized))
			}
		}
	}

	return value
}
