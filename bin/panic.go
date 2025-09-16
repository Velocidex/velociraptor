package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/mitchellh/panicwrap"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	logging "www.velocidex.com/golang/velociraptor/logging"
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
		exitStatus, err := panicwrap.BasicWrap(func(output string) {
			// Create a special log file in the log directory.
			filename := filepath.Join(
				config_obj.Logging.OutputDirectory,
				fmt.Sprintf("panic-%v.log", strings.Replace(
					time.Now().Format(time.RFC3339), ":", "_", -1)))

			fd, err := os.OpenFile(filename,
				os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				return
			}
			_, _ = fd.Write([]byte(output))
			fd.Close()
		})
		if err != nil {
			// Something went wrong setting up the panic
			// wrapper. Unlikely, but possible.
			panic(err)
		}

		// If exitStatus >= 0, then we're the parent process
		// and the panicwrap re-executed ourselves and
		// completed. Just exit with the proper status.
		if exitStatus >= 0 {
			os.Exit(exitStatus)
		}

		// Otherwise, exitStatus < 0 means we're the
		// child. Continue executing as normal...
	}
	return nil
}

func FatalIfError(command *kingpin.CmdClause, cb func() error) {
	err := cb()
	kingpin.FatalIfError(err, "%s", command.FullCommand())
}
