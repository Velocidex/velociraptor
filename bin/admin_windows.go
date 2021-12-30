// +build windows

package main

import (
	"fmt"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func checkAdmin() error {
	if *artificat_command_collect_admin_flag && !vql_subsystem.IsAdmin() {
		return fmt.Errorf("Velociraptor requires administrator level access. Use a 'Run as administrator' command shell to launch the binary.")
	}
	return nil
}
