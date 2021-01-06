package utils

import (
	"context"
	"time"

	"github.com/Velocidex/json"
	errors "github.com/pkg/errors"
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

// TODO: Deprecate this one.
type TimeVal struct {
	Sec  int64 `json:"sec"`
	Nsec int64 `json:"usec"`
}

func (self TimeVal) Time() time.Time {
	if self.Nsec > 0 {
		return time.Unix(0, self.Nsec)
	}
	return time.Unix(self.Sec, 0)
}

func (self TimeVal) Materialize(ctx context.Context, scope *vfilter.Scope) vfilter.Any {
	res, _ := self.Time().UTC().MarshalText()
	return string(res)
}

// Take care of marshaling all timestamps in UTC
func MarshalTimes(v interface{}, opts *json.EncOpts) ([]byte, error) {
	switch t := v.(type) {
	case time.Time:
		// Marshal the time in the desired timezone.
		return t.UTC().MarshalJSON()

	case *time.Time:
		return t.UTC().MarshalJSON()

	case TimeVal:
		return t.Time().UTC().MarshalJSON()

	case *TimeVal:
		return t.Time().UTC().MarshalJSON()

	}
	return nil, json.EncoderCallbackSkip
}

func init() {
	vjson.RegisterCustomEncoder(time.Time{}, MarshalTimes)
	vjson.RegisterCustomEncoder(&time.Time{}, MarshalTimes)
	vjson.RegisterCustomEncoder(TimeVal{}, MarshalTimes)
	vjson.RegisterCustomEncoder(&TimeVal{}, MarshalTimes)
}

func AnyToTime(v interface{}) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t.UTC(), nil

	case *time.Time:
		return t.UTC(), nil

	case int64:
		return time.Unix(t, 0).UTC(), nil

	case int:
		return time.Unix(int64(t), 0).UTC(), nil

	case float64:
		return time.Unix(int64(t), 0).UTC(), nil

	case uint64:
		return time.Unix(int64(t), 0).UTC(), nil
	}

	return time.Time{}, errors.New("Can not convert to time")
}
