// +build !windows

package vql

import "syscall"

func IsAdmin() bool {
	return syscall.Geteuid() == 0
}
