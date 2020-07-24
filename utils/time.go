package utils

import (
	"time"

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

func (self TimeVal) MarshalJSON() ([]byte, error) {
	return self.Time().MarshalJSON()
}
