//go:build windows
// +build windows

package main

import (
	"fmt"
	"syscall"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows/svc/eventlog"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
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

type EventlogHook struct {
	logger *eventlog.Log
}

func (self *EventlogHook) Fire(entry *log.Entry) error {
	msg, err := entry.String()
	if err != nil {
		fmt.Printf("Eror: %v\n", err)
		return nil
	}
	return self.logger.Info(1000, msg)
}

func (self *EventlogHook) Levels() []log.Level {
	return []log.Level{log.InfoLevel, log.WarnLevel, log.ErrorLevel}
}

func InstallAuditlogger() error {
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
	hook := &EventlogHook{
		logger: logger,
	}

	logging.Manager().AddHook(hook, &logging.Audit)
	return nil
}
