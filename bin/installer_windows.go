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
	"sync"
	"time"

	"context"

	errors "github.com/pkg/errors"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	service_command = app.Command(
		"service", "Manipulate the Velociraptor service.")
	installl_command = service_command.Command(
		"install", "Install Velociraptor as a Windows service.")

	remove_command = service_command.Command(
		"remove", "Remove the Velociraptor Windows service.")

	start_command = service_command.Command(
		"start", "Start the service")

	stop_command = service_command.Command(
		"stop", "Stop the service")

	pause_command = service_command.Command(
		"pause", "Pause the service")

	continue_command = service_command.Command(
		"continue", "Continue the service")

	run_command = service_command.Command(
		"run", "Run as a service - only called by service manager.").Hidden()
)

func doInstall(config_obj *api_proto.Config) (err error) {
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
		err = controlService(service_name, svc.Stop, svc.Stopped)
		if err != nil {
			logger.Info("Error stopping service %v. "+
				"Will attempt to continue anyway.", err)
		} else {
			logger.Info("Stopped service %s", service_name)
		}
	}

	// Try to copy the executable to the target_path.
	err = utils.CopyFile(executable, target_path, 0755)
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		dirname := filepath.Dir(target_path)
		logger.Info("Attempting to create intermediate directory %s.",
			dirname)
		err = os.MkdirAll(dirname, 0700)
		if err != nil {
			logger.Info("MkdirAll %s: %v", dirname, err)
			return errors.Wrap(err, "Create intermediate directories")
		}
		err = utils.CopyFile(executable, target_path, 0755)
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

		err = utils.CopyFile(*config_path, config_target_path, 0755)
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

func checkServiceExists(name string) (bool, error) {
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

func installService(
	config_obj *api_proto.Config,
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

func startService(name string) error {
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

func controlService(name string, c svc.Cmd, to svc.State) error {
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

func removeService(name string) error {
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

func doRemove() {
	config_obj, err := config.LoadClientConfig(*config_path)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to load config file")
	}

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	service_name := config_obj.Client.WindowsInstaller.ServiceName

	// Ensure the service is stopped first.
	err = controlService(service_name, svc.Stop, svc.Stopped)
	if err != nil {
		logger.Info("Could not stop service %s: %v", service_name, err)
	} else {
		logger.Info("Stopped service %s", service_name)
	}

	err = removeService(service_name)
	kingpin.FatalIfError(err, "Unable to remove service")
	logger.Info("Removed service %s", service_name)
}

func getLogger(name string) (debug.Log, error) {
	var elog debug.Log
	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		return nil, err
	}
	if isIntSess {
		elog = debug.New(name)
	} else {
		elog, err = eventlog.Open(name)
		if err != nil {
			return nil, err
		}
	}

	return elog, nil
}

func loadClientConfig() (*api_proto.Config, error) {
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

	config_obj, err := config.LoadClientConfig(*config_path)
	if err != nil {
		return nil, err
	}

	// Make sure the config is ok.
	err = crypto.VerifyConfig(config_obj)
	if err != nil {
		return nil, err
	}

	return config_obj, nil
}

func doRun() error {
	name := "Velociraptor"
	config_obj, err := loadClientConfig()
	if err == nil {
		name = config_obj.Client.WindowsInstaller.ServiceName
	}
	service, err := NewVelociraptorService(name)
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

type VelociraptorService struct {
	mu    sync.Mutex
	comms *http_comms.HTTPCommunicator
	name  string
}

func (self *VelociraptorService) SetPause(value bool) {
	self.mu.Lock()
	defer self.mu.Unlock()
	if self.comms != nil {
		self.comms.SetPause(value)
	}
}

func (self *VelociraptorService) Execute(args []string,
	r <-chan svc.ChangeRequest,
	changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}

	// Start running and tell the SCM about it.
	self.SetPause(false)
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

func (self *VelociraptorService) Close() {
	elog, err := getLogger(self.name)
	if err == nil {
		elog.Info(1, fmt.Sprintf("%s service stopped", self.name))
		elog.Close()
	}
}

func NewVelociraptorService(name string) (*VelociraptorService, error) {
	elog, err := getLogger(name)
	if err != nil {
		return nil, err
	}

	result := &VelociraptorService{name: name}

	go func() {
		for {
			// Spin forever waiting for a config file to be
			// dropped into place.
			config_obj, err := loadClientConfig()
			if err != nil {
				time.Sleep(10 * time.Second)
				continue
			}

			manager, err := crypto.NewClientCryptoManager(
				config_obj, []byte(config_obj.Writeback.PrivateKey))
			if err != nil {
				elog.Error(1, fmt.Sprintf(
					"Can not create crypto: %v", err))
				time.Sleep(10 * time.Second)
				continue
			}

			exe, err := executor.NewClientExecutor(config_obj)
			if err != nil {
				elog.Error(1, fmt.Sprintf(
					"Can not create client: %v", err))
				time.Sleep(10 * time.Second)
				continue
			}

			comm, err := http_comms.NewHTTPCommunicator(
				config_obj,
				manager,
				exe,
				config_obj.Client.ServerUrls,
			)
			if err != nil {
				elog.Error(1, fmt.Sprintf(
					"Can not create comms: %v", err))
				time.Sleep(10 * time.Second)
				continue
			}

			result.mu.Lock()
			result.comms = comm
			result.mu.Unlock()

			ctx := context.Background()
			comm.Run(ctx)
			return
		}
	}()

	return result, nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		var err error
		switch command {
		case installl_command.FullCommand():
			config_obj, err := config.LoadClientConfig(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")
			logger := logging.GetLogger(config_obj, &logging.ClientComponent)

			// Try 5 times to install the service.
			for i := 0; i < 5; i++ {
				err = doInstall(config_obj)
				if err == nil {
					break
				}
				logger.Info("%v", err)
				time.Sleep(10 * time.Second)
			}

		case remove_command.FullCommand():
			doRemove()

		case run_command.FullCommand():
			name := "velociraptor"
			err = doRun()
			if err != nil {
				elog, err := getLogger(name)
				kingpin.FatalIfError(err, "Unable to get logger")
				defer elog.Close()

				elog.Info(1, fmt.Sprintf(
					"Failed to start service: %v", err))
			}

		case start_command.FullCommand():
			config_obj, err := config.LoadClientConfig(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")
			err = startService(config_obj.Client.WindowsInstaller.ServiceName)

		case stop_command.FullCommand():
			config_obj, err := config.LoadClientConfig(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")
			err = controlService(
				config_obj.Client.WindowsInstaller.ServiceName,
				svc.Stop, svc.Stopped)

		case pause_command.FullCommand():
			config_obj, err := config.LoadClientConfig(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")
			err = controlService(
				config_obj.Client.WindowsInstaller.ServiceName,
				svc.Pause, svc.Paused)

		case continue_command.FullCommand():
			config_obj, err := config.LoadClientConfig(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")
			err = controlService(
				config_obj.Client.WindowsInstaller.ServiceName,
				svc.Continue, svc.Running)
		default:
			return false
		}

		kingpin.FatalIfError(err, "")
		return true
	})
}
