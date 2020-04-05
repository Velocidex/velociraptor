package utils

import (
	"time"

	"www.velocidex.com/golang/vfilter"
)

// Sadly Go does not have a reliable way to force JSON output on
// time.Time to be in UTC. Theoretically you can set the TZ=UTC value
// but it is impossible to reliably set it within the program itself
// since the order of initialization is undefined. If anyone
// initializes the time package before the TZ environment variable is
// set, then time.Time will be serialized in local time.

type Time struct {
	time.Time
}

func (self Time) MarshalJSON() ([]byte, error) {
	return self.Time.UTC().MarshalJSON()
}

func (self Time) Before(x Time) bool {
	return self.Time.Before(x.Time)
}

func Unix(sec, usec int64) Time {
	return Time{time.Unix(sec, usec)}
}

func IsTime(a vfilter.Any) (Time, bool) {
	switch t := a.(type) {

	case Time:
		return t, true
	case time.Time:
		return Time{t}, true
	default:
		return Unix(0, 0), false
	}
}
