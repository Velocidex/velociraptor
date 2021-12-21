// +build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	global_mutant_name = app.Flag("mutant", "When specified we use this mutant to ensure only one copy of the client is allowed to run.").String()

	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex = kernel32.NewProc("CreateMutexW")
)

// This is useful when Velociraptor is run from GPO or another
// mechanism that may start multiple copies of it. The first copy will
// succeed and the other copied will exit.
func checkMutex() error {
	if *global_mutant_name == "" {
		return nil
	}

	_, _, err := procCreateMutex.Call(
		0,
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(*global_mutant_name))),
	)
	switch int(err.(syscall.Errno)) {
	case 0:
		return nil
	default:
		return fmt.Errorf("Unable to start because mutant is in use")
	}
}
