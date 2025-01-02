//go:build windows && cgo && amd64
// +build windows,cgo,amd64

package etw

import "github.com/Velocidex/etw"

type ETWOptions struct {
	AnyKeyword, AllKeyword uint64
	Level                  int64
	CaptureState           bool
	EnableMapInfo          bool

	// A description string to be associated with the registration.
	Description string

	RundownOptions etw.RundownOptions
}
