package utils

import (
	"time"

	"github.com/Velocidex/json"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

func ParseTimeFromInt64(t int64) time.Time {
	var sec, dec int64

	// Maybe it is in ns
	if t > 20000000000000000 { // 11 October 2603 in microsec
		dec = t

	} else if t > 20000000000000 { // 11 October 2603 in milliseconds
		dec = t * 1000

	} else if t > 20000000000 { // 11 October 2603 in seconds
		dec = t * 1000000

	} else {
		sec = t
	}

	if sec == 0 && dec == 0 {
		return time.Time{}
	}

	return time.Unix(int64(sec), int64(dec))
}

func IsTime(a vfilter.Any) (time.Time, bool) {
	switch t := a.(type) {

	case time.Time:
		return t, true
	default:
		return time.Unix(0, 0), false
	}
}

// Take care of marshaling all timestamps in UTC
func MarshalTimes(v interface{}, opts *json.EncOpts) ([]byte, error) {
	switch t := v.(type) {
	case time.Time:
		// Marshal the time in the desired timezone.
		return t.UTC().MarshalJSON()

	case *time.Time:
		return t.UTC().MarshalJSON()

	}
	return nil, json.EncoderCallbackSkip
}

func init() {
	vjson.RegisterCustomEncoder(time.Time{}, MarshalTimes)
	vjson.RegisterCustomEncoder(&time.Time{}, MarshalTimes)
}
