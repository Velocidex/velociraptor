//go:build !windows
// +build !windows

package main

import (
	"fmt"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func checkAdmin() error {
	if !vql_subsystem.IsAdmin() {
		return fmt.Errorf("Velociraptor requires administrator level access. Use 'sudo' command shell to launch the binary.")
	}
	return nil
}

func checkMutex() error { return nil }

func logArgv(argv []string) error {
	return nil
}

func InstallAuditlogger() error {
	return nil
}
