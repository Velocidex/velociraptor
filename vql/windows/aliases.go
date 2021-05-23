package windows

import (
	"golang.org/x/sys/windows"
)

type GUID windows.GUID

func UTF16ToString(in []uint16) string {
	return windows.UTF16ToString(in)
}

func UTF16FromString(in string) ([]uint16, error) {
	return windows.UTF16FromString(in)
}
