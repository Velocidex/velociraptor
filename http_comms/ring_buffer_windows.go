//go:build windows
// +build windows

package http_comms

import (
	"os"
	"syscall"
)

const (
	FILE_FLAG_DELETE_ON_CLOSE = 0x04000000
)

func createFile(filename string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(filename)
	if err != nil {
		return nil, err
	}

	handle, err := syscall.CreateFile(
		name,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.CREATE_ALWAYS,
		FILE_FLAG_DELETE_ON_CLOSE|syscall.FILE_ATTRIBUTE_HIDDEN,
		0)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(handle), filename), nil
}
