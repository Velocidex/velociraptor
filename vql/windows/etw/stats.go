//go:build windows && cgo && amd64
// +build windows,cgo,amd64

package etw

import (
	"time"

	"github.com/Velocidex/ordereddict"
)

type ProviderStat struct {
	SessionName string
	GUID        string
	Description string
	EventCount  int
	Watchers    int
	Started     time.Time
	Stats       *ordereddict.Dict
}
