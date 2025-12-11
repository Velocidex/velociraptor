//go:build windows
// +build windows

/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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

	kingpin "github.com/alecthomas/kingpin/v2"
	errors "github.com/go-errors/errors"
	"github.com/virtuald/go-paniclog"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql/tools"
)

var (
	service_command = app.Command(
		"service", "Manipulate the Velociraptor service.")

	installl_command = service_command.Command(
		"install", "Install Velociraptor as a Windows service.")

	installl_command_argv = service_command.Flag(
		"argv", "Service args (default 'service', 'run').").
		Strings()

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

func doInstall(config_obj *config_proto.Config) (err error) {
	logging.DisableLogging()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if config_obj.Client.WindowsInstaller == nil {
		kingpin.Fatalf("WindowsInstaller is not configured.")
	}

	service_name := config_obj.Client.WindowsInstaller.ServiceName
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	target_path := utils.ExpandEnv(config_obj.Client.WindowsInstaller.InstallPath)

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("unable to determine executable path: %w", err)
	}
	pres, err := checkServiceExists(service_name)
	if err != nil {
		logger.Info("checkServiceExists: %v", err)
		return errors.Wrap(err, 0)
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
	err = utils.CopyFile(ctx, executable, target_path, 0755)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		dirname := filepath.Dir(target_path)
		logger.Info("Attempting to create intermediate directory %s.",
			dirname)
		err = os.MkdirAll(dirname, 0700)
		if err != nil {
			logger.Info("MkdirAll %s: %v", dirname, err)
			return fmt.Errorf("Create intermediate directories: %w", err)
		}
		err = utils.CopyFile(ctx, executable, target_path, 0755)
	}
	if err != nil {
		logger.Info("Cant copy binary to destination %s: %v", target_path, err)
		return fmt.Errorf("Cant copy binary into destination dir: %w", err)
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
		return errors.Wrap(err, 0)
	}
	if pres {
		err = removeService(service_name)
		if err != nil {
			fmt.Errorf("Remove old service: %w", err)
		}
	}

	err = installService(config_obj, target_path, logger)
	if err != nil {
		return fmt.Errorf("Install service: %w", err)
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
	config_obj *config_proto.Config,
	executable string,
	logger *logging.LogContext) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	argv := *installl_command_argv
	if len(argv) == 0 {
		argv = []string{"service", "run"}
	}

	logger.Info("Starting service with argv %v\n", argv)

	s, err := m.CreateService(
		config_obj.Client.WindowsInstaller.ServiceName,
		executable,
		mgr.Config{
			StartType:   mgr.StartAutomatic,
			DisplayName: config_obj.Client.WindowsInstaller.ServiceName,
			Description: config_obj.Client.WindowsInstaller.ServiceDescription,
		},

		// Executable will be started with this command line args:
		argv...)

	if err != nil {
		return err
	}
	defer s.Close()

	// Set the service to autostart
	err = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: time.Second * 60},
		{Type: mgr.ServiceRestart, Delay: time.Second * 60},
		{Type: mgr.ServiceRestart, Delay: time.Second * 60},
	}, 60)
	if err != nil {
		logger.Info("SetRecoveryActions() failed: %s", err)
	}

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
		return fmt.Errorf("service %s is not installed: %w", name, err)
	}
	defer s.Close()
	err = s.Delete()
	if err != nil {
		return err
	}
	err = eventlog.Remove(name)
	if err != nil {
		return fmt.Errorf("RemoveEventLogSource() failed: %w", err)
	}
	return nil
}

func doRemove() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
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
	if err != nil {
		return fmt.Errorf("Unable to remove service: %w", err)
	}
	logger.Info("Removed service %s", service_name)
	return nil
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

// Try to emit a log to the system log file but this is not fatal - if
// we cant for some reason we just move on.
func tryToLog(name, message string) {
	elog, err := getLogger(name)
	if err != nil {
		return
	}
	defer elog.Close()

	elog.Info(1, message)
	writeToStderr(message)
}

func writeToStderr(items ...string) {
	println(time.Now().Format(time.RFC3339), ": ", strings.Join(items, " "))
}

func loadClientConfig() (*config_proto.Config, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}

	config_obj, err := makeDefaultConfigLoader().

		// If the config path is not specified we look for the config
		// file next to the service executable.
		WithFileLoader(strings.TrimSuffix(
			executable, filepath.Ext(executable)) + ".config.yaml").
		WithRequiredClient().
		WithWriteback().LoadAndValidate()
	if err != nil {
		// Config obj is not valid here, we can not actually
		// log anything since we dont know where to send it so
		// prelog instead.
		Prelog("Failed to load %v: %v will try again soon.\n", err, *config_path)
		return nil, err
	}

	tempfile.SetTempfile(config_obj)

	// Make sure the config is ok.
	err = crypto_utils.VerifyConfig(config_obj)
	if err != nil {
		Prelog("VerifyConfig: %v.\n", err)
		return nil, err
	}

	return config_obj, nil
}

func maybeWritePanicFile(name string, config_obj *config_proto.Config) {
	if config_obj.Client == nil ||
		config_obj.Client.PanicFile == "" {
		return
	}

	// Make sure %TEMP% is set correctly here
	tempfile.SetTempfile(config_obj)

	panic_file_path := utils.ExpandEnv(config_obj.Client.PanicFile)
	fd, err := os.OpenFile(panic_file_path,
		os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		Prelog("Error opening panic file: %v\n", err)
		tryToLog(name, fmt.Sprintf("Error opening panic file: %v\n", err))
		return
	}

	tryToLog(name, "Redirecting output to "+fd.Name())
	_, err = paniclog.RedirectStderr(fd)
	if err != nil {
		writeToStderr("Error redirecting stderr ", err.Error())
		return
	}

	fd.Close()
}

func doRun() error {
	name := "Velociraptor"
	config_obj, err := loadClientConfig()
	if err == nil && config_obj != nil &&
		config_obj.Client != nil &&
		config_obj.Client.WindowsInstaller != nil {
		name = config_obj.Client.WindowsInstaller.ServiceName
	}

	if config_obj != nil {
		maybeWritePanicFile(name, config_obj)
		if config_obj.Client != nil {
			config_obj.Client.PanicFile = ""
		}
	}

	if err != nil {
		writeToStderr("NewVelociraptorService:", err.Error())
	}
	ctx := context.Background()
	service, err := NewVelociraptorService(ctx, name)
	if err != nil {
		writeToStderr("NewVelociraptorService:", err.Error())
		return err
	}
	defer service.Close()

	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		writeToStderr("IsAnInteractiveSession: ", err.Error())
		return err
	}

	if isIntSess {
		err = debug.Run(name, service)
	} else {
		err = svc.Run(name, service)
	}
	if err != nil {
		writeToStderr("svc.Run: ", err.Error())
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
	const cmdsAccepted = svc.AcceptStop |
		svc.AcceptShutdown | svc.AcceptPauseAndContinue

	changes <- svc.Status{State: svc.StartPending}

	// Start running and tell the SCM about it.
	self.SetPause(false)

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	tryToLog(self.name, "Starting service "+self.name)

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
				tryToLog(self.name, "Service Paused")

			case svc.Continue:
				changes <- svc.Status{
					State:   svc.Running,
					Accepts: cmdsAccepted,
				}
				self.SetPause(false)
				tryToLog(self.name, "Service Resumed")

			default:
				tryToLog(self.name, fmt.Sprintf(
					"unexpected control request #%d", c))
			}
		}
	}

	changes <- svc.Status{State: svc.StopPending}
	tryToLog(self.name, "Service Shutting Down")
	return
}

func (self *VelociraptorService) Close() {
	tryToLog(self.name, fmt.Sprintf("%s service stopped", self.name))
}

func runOnce(ctx context.Context,
	wg *sync.WaitGroup, result *VelociraptorService, log_name string) {
	defer wg.Done()

	// Spin forever waiting for a config file to be
	// dropped into place.
	config_obj, err := loadClientConfig()
	if err != nil {
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
		}
		return
	}

	maybeWritePanicFile(log_name, config_obj)
	_ = InstallAuditlogger()

	writeback_service := writeback.GetWritebackService()
	writeback, err := writeback_service.GetWriteback(config_obj)
	if err != nil {
		return
	}

	sm, err := startup.StartClientServices(ctx, config_obj, on_error)
	defer sm.Close()

	exe, err := executor.NewClientExecutor(
		ctx, writeback.ClientId, config_obj)
	if err != nil {
		tryToLog(log_name, fmt.Sprintf(
			"Can not create client: %v", err))
		time.Sleep(10 * time.Second)
		return
	}

	_, err = http_comms.StartHttpCommunicatorService(
		ctx, sm.Wg, config_obj, exe, on_error)
	if err != nil {
		tryToLog(log_name, fmt.Sprintf(
			"Can not create client: %v", err))
		time.Sleep(10 * time.Second)
		return
	}

	// Check for crashes etc
	err = executor.RunStartupTasks(ctx, config_obj, sm.Wg, exe)
	if err != nil {
		// Not a fatal error, just move on
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Error("<red>Start up error:</> %v", err)
	}

	<-ctx.Done()
}

func NewVelociraptorService(
	ctx context.Context, name string) (*VelociraptorService, error) {
	result := &VelociraptorService{name: name}

	go func() {
		for {
			subctx, cancel := context.WithCancel(ctx)
			lwg := &sync.WaitGroup{}
			lwg.Add(1)
			go func() {
				defer cancel()
				runOnce(subctx, lwg, result, name)
			}()

			select {
			case <-subctx.Done():
				continue

			case <-ctx.Done():
				// Wait for the client to shutdown.
				cancel()
				lwg.Wait()
				return

			case <-tools.ClientRestart:
				cancel()
				lwg.Wait()
			}
		}
	}()

	return result, nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		var err error
		switch command {
		case installl_command.FullCommand():
			config_obj, err := makeDefaultConfigLoader().LoadAndValidate()
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
			FatalIfError(remove_command, doRemove)

		case run_command.FullCommand():
			name := "velociraptor"
			err = doRun()
			if err != nil {
				tryToLog(name, fmt.Sprintf(
					"Failed to start service: %v", err))
			}

		case start_command.FullCommand():
			config_obj, err := makeDefaultConfigLoader().LoadAndValidate()
			kingpin.FatalIfError(err, "Unable to load config file")
			err = startService(config_obj.Client.WindowsInstaller.ServiceName)

		case stop_command.FullCommand():
			config_obj, err := makeDefaultConfigLoader().LoadAndValidate()
			kingpin.FatalIfError(err, "Unable to load config file")
			err = controlService(
				config_obj.Client.WindowsInstaller.ServiceName,
				svc.Stop, svc.Stopped)

		case pause_command.FullCommand():
			config_obj, err := makeDefaultConfigLoader().LoadAndValidate()
			kingpin.FatalIfError(err, "Unable to load config file")
			err = controlService(
				config_obj.Client.WindowsInstaller.ServiceName,
				svc.Pause, svc.Paused)

		case continue_command.FullCommand():
			config_obj, err := makeDefaultConfigLoader().LoadAndValidate()
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

	// From now on all LoadLibrary() calls will be done from the
	// system directories. This does not help any libraries linked
	// into the binary because they would have loaded already but this
	// helps to reduce our DLL hijacking surface especially with
	// external libraries.
	windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
}
