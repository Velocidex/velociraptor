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
	Location *time.Location
}

func NewTime(t time.Time) Time {
	return Time{Time: t}
}

func (self Time) MarshalJSON() ([]byte, error) {
	if self.Location != nil {
		return self.In(self.Location).MarshalJSON()
	}

	return self.Time.UTC().MarshalJSON()
}

func (self Time) Before(x Time) bool {
	return self.Time.Before(x.Time)
}

func Unix(sec, usec int64) Time {
	return NewTime(time.Unix(sec, usec))
}

func IsTime(a vfilter.Any) (Time, bool) {
	switch t := a.(type) {

	case Time:
		return t, true
	case time.Time:
		return NewTime(t), true
	default:
		return Unix(0, 0), false
	}
}

// TODO: Deprecate this one.
type TimeVal struct {
	Sec  int64 `json:"sec"`
	Nsec int64 `json:"usec"`
}

func (self TimeVal) Time() Time {
	if self.Nsec > 0 {
		return Unix(0, self.Nsec)
	}
	return Unix(self.Sec, 0)
}

func (self TimeVal) MarshalJSON() ([]byte, error) {
	return self.Time().MarshalJSON()
}
