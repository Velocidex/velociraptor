//+ build: !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

func checkAdmin() {
	if *artificat_command_collect_admin_flag && syscall.Geteuid() != 0 {
		fmt.Println("Velociraptor requires administrator level access. Use a 'Run as administrator' command shell to launch the binary.")
		os.Exit(-1)
	}

}
