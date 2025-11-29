package main

import (
	"fmt"
	"strings"

	errors "github.com/go-errors/errors"
)

var (
	run_flag = app.Flag("run", "Run an artifact as a CLI tool.").
			Short('r').String()

	stopError = errors.New("Stop")
)

// Detect alternative command line processing and transform into a
// standardized set.
func transformArgv(argv []string) ([]string, error) {
	for idx, arg := range argv {

		// We are in run mode - any following parameters will be
		// interpreted as CLI args to the artifact.
		if idx < len(argv)-1 && (arg == "-r" || arg == "--run") {
			artifact_name := argv[idx+1]
			result := append([]string{
				"artifacts", "collect", artifact_name}, argv[:idx]...)

			// Expected the rest of the args to follow artifact params
			for i := idx; i < len(argv); i++ {
				arg := argv[i]

				// CLI help mode
				if arg == "-h" || arg == "--help" {
					result := append([]string{},
						[]string{"artifacts", "collect",
							"--cli_help_mode", artifact_name}...)

					result = append(result, argv[:idx]...)
					return result, nil
				}

				// The arg can contain = to separate the name and
				// value
				if strings.Contains(arg, "=") {
					parts := strings.SplitN(arg, "=", 2)
					result = append(result,
						[]string{"--args", parts[0][2:] + "=" + parts[1]}...)
					continue
				}

				if strings.HasPrefix(arg, "--") {
					if i+1 >= len(argv) {
						return nil, fmt.Errorf("Parameter %v must be followed by value",
							arg)
					}
					param := argv[i+1]

					// Does the parameter have an = sign in it?
					result = append(result,
						[]string{"--args", arg[2:] + "=" + param}...)
				}
			}
			return result, nil
		}
	}
	return argv, nil
}
