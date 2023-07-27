package psutils

import (
	"errors"
	"os"
)

var (
	ErrorProcessNotRunning = errors.New("ErrorProcessNotRunning")
	ErrorNotPermitted      = errors.New("operation not permitted")
	NotImplementedError    = errors.New("NotImplementedError")
)

type TimesStat struct {
	User   float64 `json:"user"`
	System float64 `json:"system"`
}

type MemoryInfoStat struct {
	RSS  uint64 `json:"rss"`  // bytes
	VMS  uint64 `json:"vms"`  // bytes
	Swap uint64 `json:"swap"` // bytes
}

type IOCountersStat struct {
	ReadCount  uint64 `json:"readCount"`
	WriteCount uint64 `json:"writeCount"`
	ReadBytes  uint64 `json:"readBytes"`
	WriteBytes uint64 `json:"writeBytes"`
}

// This works on all OSs
func Kill(pid int32) error {
	process, err := os.FindProcess(int(pid))
	if err != nil {
		return err
	}
	return process.Kill()
}
