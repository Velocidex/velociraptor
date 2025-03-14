//go:build windows && cgo && amd64
// +build windows,cgo,amd64

package etw

import (
	"strings"

	"github.com/Velocidex/etw"
)

type ETWOptions struct {
	AnyKeyword, AllKeyword uint64
	Level                  int64
	CaptureState           bool
	EnableMapInfo          bool

	// A description string to be associated with the registration.
	Description string

	RundownOptions etw.RundownOptions
}

func optionsString(in etw.RundownOptions) string {
	res := []string{}
	if in.Registry {
		res = append(res, "Registry")
	}

	if in.Process {
		res = append(res, "Process")
	}

	if in.ImageLoad {
		res = append(res, "ImageLoad")
	}

	if in.Network {
		res = append(res, "Network")
	}

	if in.Driver {
		res = append(res, "Driver")
	}

	if in.File {
		res = append(res, "File")
	}

	if in.Thread {
		res = append(res, "Thread")
	}

	if in.Handles {
		res = append(res, "Handles")
	}

	return strings.Join(res, ", ")
}

func megrgeRundown(out *etw.RundownOptions, in etw.RundownOptions) {
	if in.Registry {
		out.Registry = true
	}

	if in.Process {
		out.Process = true
	}

	if in.ImageLoad {
		out.ImageLoad = true
	}

	if in.Network {
		out.Network = true
	}

	if in.Driver {
		out.Driver = true
	}

	if in.File {
		out.File = true
	}

	if in.Thread {
		out.Thread = true
	}

	if in.Handles {
		out.Handles = true
	}

}
