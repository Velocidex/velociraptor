//go:build windows && cgo && (amd64 || arm64)
// +build windows
// +build cgo
// +build amd64 arm64

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
