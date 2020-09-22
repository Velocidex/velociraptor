// +build windows

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"context"

	errors "github.com/pkg/errors"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	server_service_command = app.Command(
		"server_service", "Manipulate the Velociraptor service.")
	server_service_installl_command = server_service_command.Command(
		"install", "Install Velociraptor frontend as a Windows service.")

	server_service_remove_command = server_service_command.Command(
		"remove", "Remove the Velociraptor Windows service.")

	server_service_start_command = server_service_command.Command(
		"start", "Start the service")

	server_service_stop_command = server_service_command.Command(
		"stop", "Stop the service")

	server_service_pause_command = server_service_command.Command(
		"pause", "Pause the service")

	server_service_continue_command = server_service_command.Command(
		"continue", "Continue the service")

	server_service_run_command = server_service_command.Command(
		"run", "Run as a service - only called by service manager.").Hidden()
)

func doInstallServerService(config_obj *config_proto.Config) (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	service_name := config_obj.Client.WindowsInstaller.ServiceName
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	target_path := os.ExpandEnv(config_obj.Client.WindowsInstaller.InstallPath)

	executable, err := os.Executable()
	kingpin.FatalIfError(err, "unable to determine executable path")
	pres, err := checkServiceExists(service_name)
	if err != nil {
		logger.Info("checkServiceExists: %v", err)
		return errors.WithStack(err)
	}
	if pres {
		// We have to stop the service first, or we can not overwrite the file.
		err = controlServiceServerService(
			service_name, svc.Stop, svc.Stopped)
		if err != nil {
			logger.Info("Error stopping service %v. "+
				"Will attempt to continue anyway.", err)
		} else {
			logger.Info("Stopped service %s", service_name)
		}
	}

	// Try to copy the executable to the target_path.
	err = utils.CopyFile(ctx, executable, target_path, 0755)
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		dirname := filepath.Dir(target_path)
		logger.Info("Attempting to create intermediate directory %s.",
			dirname)
		err = os.MkdirAll(dirname, 0700)
		if err != nil {
			logger.Info("MkdirAll %s: %v", dirname, err)
			return errors.Wrap(err, "Create intermediate directories")
		}
		err = utils.CopyFile(ctx, executable, target_path, 0755)
	}
	if err != nil {
		logger.Info("Cant copy binary to destination %s: %v", target_path, err)
		return errors.Wrap(err, "Cant copy binary into destination dir.")
	}

	logger.Info("Copied binary to %s", target_path)

	// If the installer was invoked with the --config arg then we
	// need to copy the config to the target path.
	if *config_path != "" {
		config_target_path := strings.TrimSuffix(
			target_path, filepath.Ext(target_path)) + ".config.yaml"

		logger.Info("Copying config to destination %s",
			config_target_path)

		err = utils.CopyFile(ctx, *config_path, config_target_path, 0755)
		if err != nil {
			logger.Info("Cant copy config to destination %s: %v",
				config_target_path, err)
			return err
		}
	}

	// A service already exists - we need to delete it and
	// recreate it to make sure it is set up correctly.
	pres, err = checkServiceExists(service_name)
	if err != nil {
		logger.Info("checkServiceExists: %v", err)
		return errors.WithStack(err)
	}
	if pres {
		err = removeService(service_name)
		if err != nil {
			errors.Wrap(err, "Remove old service")
		}
	}

	err = installService(config_obj, target_path, logger)
	if err != nil {
		return errors.Wrap(err, "Install service")
	}

	logger.Info("Installed service %s", service_name)

	// Since we stopped the service here, we need to make sure it
	// is started again.
	err = startService(service_name)

	// We can not start the service - everything is messed
	// up! Just die here.
	if err != nil {
		return
	}
	logger.Info("Started service %s", service_name)

	return nil
}

func checkServiceExistsServerService(name string) (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err == nil {
		s.Close()
		return true, nil
	}

	return false, nil
}

func installServiceServerService(
	config_obj *config_proto.Config,
	executable string,
	logger *logging.LogContext) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.CreateService(
		config_obj.Client.WindowsInstaller.ServiceName,
		executable,
		mgr.Config{
			StartType:   mgr.StartAutomatic,
			DisplayName: config_obj.Client.WindowsInstaller.ServiceName,
			Description: config_obj.Client.WindowsInstaller.ServiceDescription,
		},

		// Executable will be started with this command line args:
		"service", "run")
	if err != nil {
		return err
	}
	defer s.Close()

	// Try to create an event source but dont sweat it if it does
	// not work.
	err = eventlog.InstallAsEventCreate(
		"velociraptor", eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		logger.Info("SetupEventLogSource() failed: %s", err)
	}
	return nil
}

func startServiceServerService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	err = s.Start("service", "run")
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func controlServiceServerService(name string, c svc.Cmd, to svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Control(c)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != to {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}

func removeServiceServerService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return errors.New(fmt.Sprintf("service %s is not installed", name))
	}
	defer s.Close()
	err = s.Delete()
	if err != nil {
		return err
	}
	err = eventlog.Remove(name)
	if err != nil {
		return errors.New(fmt.Sprintf("RemoveEventLogSource() failed: %s", err))
	}
	return nil
}

func doRemoveServerService() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	service_name := config_obj.Client.WindowsInstaller.ServiceName

	// Ensure the service is stopped first.
	err = controlServiceServerService(
		service_name, svc.Stop, svc.Stopped)
	if err != nil {
		logger.Info("Could not stop service %s: %v", service_name, err)
	} else {
		logger.Info("Stopped service %s", service_name)
	}

	err = removeService(service_name)
	kingpin.FatalIfError(err, "Unable to remove service")
	logger.Info("Removed service %s", service_name)
}

func loadServerConfig() (*config_proto.Config, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}

	// If the config path is not specified we look for the config
	// file next to the service executable.
	if *config_path == "" {
		config_target_path := strings.TrimSuffix(
			executable, filepath.Ext(executable)) + ".config.yaml"
		config_path = &config_target_path
	}

	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return nil, err
	}

	return config_obj, nil
}

func doRunServerService() error {
	name := "Velociraptor"
	config_obj, err := loadServerConfig()
	if err == nil {
		name = config_obj.Client.WindowsInstaller.ServiceName
	}
	service, err := NewVelociraptorServerService(name)
	if err != nil {
		return err
	}
	defer service.Close()

	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		return err
	}

	if isIntSess {
		err = debug.Run(name, service)
	} else {
		err = svc.Run(name, service)
	}
	if err != nil {
		return err
	}

	return nil
}

type VelociraptorServerService struct {
	name string
}

func (self *VelociraptorServerService) SetPause(value bool) {
}

func (self *VelociraptorServerService) Execute(args []string,
	r <-chan svc.ChangeRequest,
	changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}

	// Start running and tell the SCM about it.
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	elog, err := getLogger(self.name)
	if err != nil {
		return
	}
	defer elog.Close()

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				break loop
			case svc.Pause:
				changes <- svc.Status{
					State:   svc.Paused,
					Accepts: cmdsAccepted,
				}
				self.SetPause(true)
				elog.Info(1, "Service Paused")

			case svc.Continue:
				changes <- svc.Status{
					State:   svc.Running,
					Accepts: cmdsAccepted,
				}
				self.SetPause(false)
				elog.Info(1, "Service Resumed")

			default:
				elog.Error(1, fmt.Sprintf(
					"unexpected control request #%d", c))
			}
		}
	}

	changes <- svc.Status{State: svc.StopPending}
	elog.Info(1, "Service Shutting Down")
	return
}

func (self *VelociraptorServerService) Close() {
	elog, err := getLogger(self.name)
	if err == nil {
		elog.Info(1, fmt.Sprintf("%s service stopped", self.name))
		elog.Close()
	}
}

func NewVelociraptorServerService(name string) (
	*VelociraptorServerService, error) {
	result := &VelociraptorServerService{name: name}

	elog, err := getLogger(name)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			elog.Info(1, "Loading service\n")
			// Spin forever waiting for a config file to be
			// dropped into place.
			config_obj, err := loadServerConfig()
			if err != nil {
				elog.Info(1, fmt.Sprintf(
					"Unable to load config: %v", err))
				time.Sleep(10 * time.Second)
				continue
			}

			ctx, cancel := install_sig_handler()
			defer cancel()

			sm := services.NewServiceManager(ctx, config_obj)
			defer sm.Close()

			elog.Info(1, fmt.Sprintf("%s service started", name))
			server, err := startFrontend(sm)
			if err != nil {
				elog.Info(1, fmt.Sprintf("%s service error", err))
				return
			}
			defer server.Close()

			// Wait here until everything is done.
			sm.Wg.Wait()

			return
		}
	}()

	return result, nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		var err error

		loader := DefaultConfigLoader.WithRequiredFrontend()

		switch command {
		case server_service_installl_command.FullCommand():
			config_obj, err := loader.LoadAndValidate()
			kingpin.FatalIfError(err, "Unable to load config file")
			logger := logging.GetLogger(config_obj, &logging.ClientComponent)

			// Try 5 times to install the service.
			for i := 0; i < 5; i++ {
				err = doInstallServerService(config_obj)
				if err == nil {
					break
				}
				logger.Info("%v", err)
				time.Sleep(10 * time.Second)
			}

		case server_service_remove_command.FullCommand():
			doRemoveServerService()

		case server_service_run_command.FullCommand():
			name := "velociraptor"
			err = doRunServerService()
			if err != nil {
				elog, err := getLogger(name)
				kingpin.FatalIfError(err, "Unable to get logger")
				defer elog.Close()

				elog.Info(1, fmt.Sprintf(
					"Failed to start service: %v", err))
			}

		case server_service_start_command.FullCommand():
			config_obj, err := loader.LoadAndValidate()
			kingpin.FatalIfError(err, "Unable to load config file")
			err = startService(config_obj.Client.WindowsInstaller.ServiceName)

		case server_service_stop_command.FullCommand():
			config_obj, err := loader.LoadAndValidate()
			kingpin.FatalIfError(err, "Unable to load config file")
			err = controlServiceServerService(
				config_obj.Client.WindowsInstaller.ServiceName,
				svc.Stop, svc.Stopped)

		case server_service_pause_command.FullCommand():
			config_obj, err := loader.LoadAndValidate()
			kingpin.FatalIfError(err, "Unable to load config file")
			err = controlServiceServerService(
				config_obj.Client.WindowsInstaller.ServiceName,
				svc.Pause, svc.Paused)

		case server_service_continue_command.FullCommand():
			config_obj, err := loader.LoadAndValidate()
			kingpin.FatalIfError(err, "Unable to load config file")
			err = controlServiceServerService(
				config_obj.Client.WindowsInstaller.ServiceName,
				svc.Continue, svc.Running)
		default:
			return false
		}

		kingpin.FatalIfError(err, "")
		return true
	})
}
