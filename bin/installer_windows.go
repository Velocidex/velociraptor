// +build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"context"

	errors "github.com/pkg/errors"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
	"gopkg.in/alecthomas/kingpin.v2"
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

func doInstall() error {
	config_obj, err := config.LoadClientConfig(*config_path)
	if err != nil {
		return errors.Wrap(err, "Unable to load config file")
	}

	service_name := config_obj.Client.WindowsInstaller.ServiceName
	logger := logging.NewLogger(config_obj)

	target_path := os.ExpandEnv(config_obj.Client.WindowsInstaller.InstallPath)

	executable, err := os.Executable()
	kingpin.FatalIfError(err, "Can't get executable path")

	pres, err := checkServiceExists(service_name)
	if err != nil {
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

	// Since we stopped the service here, we need to make sure it
	// is started again.
	defer func() {
		err = startService(service_name)

		// We can not start the service - everything is messed
		// up! Just die here.
		kingpin.FatalIfError(err, "Start service")

		logger.Info("Started service %s", service_name)
	}()

	// Try to copy the executable to the target_path.
	err = utils.CopyFile(executable, target_path, 0755)
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		dirname := filepath.Dir(target_path)
		logger.Info("Attempting to create intermediate directory %s.",
			dirname)
		err = os.MkdirAll(dirname, 0700)
		if err != nil {
			return errors.Wrap(err, "Create intermediate directories")
		}
		err = utils.CopyFile(executable, target_path, 0755)
	}
	if err != nil {
		return errors.Wrap(err, "Cant copy binary into destination dir.")
	}

	logger.Info("Copied binary to %s", target_path)

	// A service already exists - we need to delete it and
	// recreate it to make sure it is set up correctly.
	pres, err = checkServiceExists(service_name)
	if err != nil {
		return errors.WithStack(err)
	}
	if pres {
		err = removeService(service_name)
		if err != nil {
			errors.Wrap(err, "Remove old service")
		}
	}

	err = installService(service_name, target_path, logger)
	if err != nil {
		return errors.Wrap(err, "Install service")
	}

	logger.Info("Installed service %s", service_name)

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

func installService(name string, executable string, logger *logging.Logger) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.CreateService(name, executable,
		mgr.Config{
			StartType:   mgr.StartAutomatic,
			DisplayName: name,
			Description: "Velociraptor service",
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
		name, eventlog.Error|eventlog.Warning|eventlog.Info)
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

	logger := logging.NewLogger(config_obj)
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

func doRun() {
	config_obj, err := config.LoadClientConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config file")

	// Make sure the config is ok.
	err = crypto.VerifyConfig(config_obj)
	kingpin.FatalIfError(err, "Unable to load config file")

	name := config_obj.Client.WindowsInstaller.ServiceName

	var elog debug.Log
	run := svc.Run

	isIntSess, err := svc.IsAnInteractiveSession()
	kingpin.FatalIfError(err,
		"failed to determine if we are running in an interactive session")
	if isIntSess {
		elog = debug.New(name)
		run = debug.Run
	} else {
		elog, err = eventlog.Open(name)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	service, err := NewVelociraptorService(config_obj, elog)
	if err != nil {
		elog.Error(1, fmt.Sprintf("%s service failed: %v", name, err))
		return
	}

	err = run(name, service)
	if err != nil {
		elog.Error(1, fmt.Sprintf("%s service failed: %v", name, err))
		return
	}
	elog.Info(1, fmt.Sprintf("%s service stopped", name))
}

type VelociraptorService struct {
	config_obj *config.Config
	ctx        context.Context
	comms      *http_comms.HTTPCommunicator
	elog       debug.Log
}

func (self *VelociraptorService) Execute(args []string,
	r <-chan svc.ChangeRequest,
	changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}

	// Start running and tell the SCM about it.
	self.comms.SetPause(false)
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

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
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
				self.comms.SetPause(true)
				self.elog.Info(1, "Service Paused")

			case svc.Continue:
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
				self.comms.SetPause(false)
				self.elog.Info(1, "Service Resumed")

			default:
				self.elog.Error(1, fmt.Sprintf("unexpected control request #%d", c))
			}
		}
	}

	changes <- svc.Status{State: svc.StopPending}
	self.elog.Info(1, "Service Shutting Down")
	return
}

func NewVelociraptorService(config_obj *config.Config, elog debug.Log) (
	*VelociraptorService, error) {
	result := &VelociraptorService{
		elog: elog,
	}

	manager, err := crypto.NewClientCryptoManager(
		config_obj, []byte(config_obj.Writeback.PrivateKey))
	if err != nil {
		return nil, err
	}

	exe, err := executor.NewClientExecutor(config_obj)
	if err != nil {
		return nil, err
	}

	comm, err := http_comms.NewHTTPCommunicator(
		config_obj,
		manager,
		exe,
		config_obj.Client.ServerUrls,
	)
	if err != nil {
		return nil, err
	}

	result.comms = comm

	// Dont actually do anything until the service manager tells
	// us to start.
	result.comms.SetPause(true)

	go func() {
		ctx := context.Background()
		comm.Run(ctx)
	}()

	return result, nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		var err error
		switch command {
		case "service install":
			err = doInstall()
		case "service remove":
			doRemove()

		case "service run":
			doRun()

		case "service start":
			config_obj, err := config.LoadClientConfig(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")
			err = startService(config_obj.Client.WindowsInstaller.ServiceName)

		case "service stop":
			config_obj, err := config.LoadClientConfig(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")
			err = controlService(
				config_obj.Client.WindowsInstaller.ServiceName,
				svc.Stop, svc.Stopped)

		case "service pause":
			config_obj, err := config.LoadClientConfig(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")
			err = controlService(
				config_obj.Client.WindowsInstaller.ServiceName,
				svc.Pause, svc.Paused)

		case "service continue":
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
