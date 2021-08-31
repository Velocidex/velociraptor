package utils

import (
	"time"

	"github.com/Velocidex/json"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

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
