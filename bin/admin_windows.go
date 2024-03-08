//go:build windows
// +build windows

package main

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/windows/svc/eventlog"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func checkAdmin() error {
	if !vql_subsystem.IsAdmin() {
		return fmt.Errorf("Velociraptor requires administrator level access. Use a 'Run as administrator' command shell to launch the binary.")
	}
	return nil
}

// The code below is from
// https://github.com/golang/go/issues/59780#issuecomment-1522252387
var (
	nullString, _ = syscall.UTF16PtrFromString("")
	neLogOemCode  = uint32(1000)
)

func logArgv(argv []string) error {
	source_name := "Velociraptor"
	err := eventlog.InstallAsEventCreate(
		source_name, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		// If we fail to register our own source (maybe due to
		// permission errors) we steal the .NET source
		source_name = ".NET Runtime"
	}

	logger, err := eventlog.Open(source_name)
	if err != nil {
		return err
	}
	defer logger.Close()

	msg := fmt.Sprintf("Velociraptor startup ARGV: %v\n",
		json.MustMarshalString(argv))

	return logger.Info(1000, msg)
}
