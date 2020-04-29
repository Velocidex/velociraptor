package utils

import "time"

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (self RealClock) Now() time.Time {
	return time.Now()
}

type MockClock struct {
	MockNow time.Time
}

func (self MockClock) Now() time.Time {
	return self.MockNow
}
