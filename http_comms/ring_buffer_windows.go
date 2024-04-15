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

	// For tests we just create a regular file.
	if PREPARE_FOR_TESTS {
		return os.OpenFile(filename,
			os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	}

	// In production we create a file with exclusive access that will
	// be deleted when closed.
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
