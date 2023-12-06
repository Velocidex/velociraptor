//go:build windows && cgo
// +build windows,cgo

package etw

import "time"

type ProviderStat struct {
	SessionName string
	GUID        string
	Description string
	EventCount  int
	Watchers    int
	Started     time.Time
}
