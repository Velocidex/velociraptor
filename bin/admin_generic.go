// +build !windows

package main

import (
	"fmt"
	"syscall"
)

func checkAdmin() error {
	if *artificat_command_collect_admin_flag && syscall.Geteuid() != 0 {
		return fmt.Errorf("Velociraptor requires administrator level access. Use 'sudo' command shell to launch the binary.")
	}
	return nil
}

func checkMutex() error { return nil }
