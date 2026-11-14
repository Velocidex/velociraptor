package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/grongor/panicwatch"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

func writeLogOnPanic() error {
	// Figure out the log directory.
	config_obj, err := new(config.Loader).
		WithFileLoader(*config_path).
		WithEmbedded(*embedded_config_path).
		WithEnvLiteralLoader(constants.VELOCIRAPTOR_LITERAL_CONFIG).
		WithEnvLoader(constants.VELOCIRAPTOR_CONFIG).
		LoadAndValidate()
	if err != nil {
		logging.FlushPrelogs(config.GetDefaultConfig())
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	if config_obj.Logging != nil &&
		config_obj.Logging.OutputDirectory != "" {

		err := panicwatch.Start(panicwatch.Config{
			OnPanic: func(p panicwatch.Panic) {
				// Create a special log file in the log directory.
				filename := utils.Join(
					config_obj.Logging.OutputDirectory,
					fmt.Sprintf("panic-%v.log", strings.Replace(
						time.Now().Format(time.RFC3339), ":", "_", -1)))

				fd, err := os.OpenFile(filename,
					os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
				if err != nil {
					return
				}
				// Try our best to write - cant do anything if this does not work.
				_, _ = fmt.Fprintf(fd, "Panic: %v\n Stack: %v", p.Message, p.Stack)
				fd.Close()
			},
			OnWatcherDied: func(err error) {
				logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
				logger.Error("panicwatch watcher process died!")
			},
		})
		if err != nil {
			// Something went wrong setting up the panic
			// wrapper. Unlikely, but possible.
			panic(err)
		}
	}
	return nil
}

func FatalIfError(command *kingpin.CmdClause, cb func() error) {
	err := cb()
	kingpin.FatalIfError(err, "%s", command.FullCommand())
}
