// +build windows

package windows

import (
	"golang.org/x/sys/windows"
)

type GUID windows.GUID

func (self GUID) String() string {
	return windows.GUID(self).String()
}

func UTF16ToString(in []uint16) string {
	return windows.UTF16ToString(in)
}

func UTF16FromString(in string) ([]uint16, error) {
	return windows.UTF16FromString(in)
}

func UTF16PtrFromString(in string) (*uint16, error) {
	return windows.UTF16PtrFromString(in)
}
