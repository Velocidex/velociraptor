package main

import (
	"fmt"
	"strings"

	errors "github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	run_flag = app.Flag("run", "Run an artifact as a CLI tool.").
			Short('r').String()

	stopError = errors.New("Stop")
)

type STATE int

type StateStack struct {
	states []STATE
}

func (self *StateStack) State() STATE {
	return self.states[len(self.states)-1]
}

func (self *StateStack) Push(state STATE) {
	self.states = append(self.states, state)
}

func (self *StateStack) Pop() {
	self.states = self.states[:len(self.states)-1]
}

const (
	_ = iota

	START_MODE STATE = iota

	// This state is triggered when we see a flag that belongs to the
	// artifact_command_collect command. We ignore it and the next arg
	// to ensure it passes undisturbed to the parser.
	RUN_CLI_MODE

	// We saw the -r flag - next look for the artifact name.
	RUN_ARTIFACT_MODE

	// Collecting args for run mode
	RUN_ARGS_MODE

	RUN_ARGS_VALUE_MODE
)

// Detect alternative command line processing and transform into a
// standardized set.
func transformArgv(argv []string) ([]string, error) {
	var prefix []string
	var runmode_args []string

	// Get all flags that belong to the `artifact collect` command.
	choices, _, _ := artifact_command_collect.FlagCompletion("", "")

	// When parsing the artifact arg we hold this until we get the
	// next value.
	var current_artifact_arg string

	// The name of the artifact.
	var artifact_name string

	state := StateStack{}
	state.Push(START_MODE)

	for _, arg := range argv {
		switch state.State() {
		case START_MODE:
			if arg == "-r" || arg == "--run" {
				runmode_args = []string{"artifacts", "collect"}
				state.Push(RUN_ARTIFACT_MODE)
				continue
			}

			// Regular args, just append them to the prefix
			prefix = append(prefix, arg)

		case RUN_ARTIFACT_MODE:
			// Next parameter is the artifact name.
			artifact_name = arg
			runmode_args = append(runmode_args, artifact_name)

			// Next collect artifact parameters
			state.Push(RUN_ARGS_MODE)

			// Accept the arg parameter for the artifact.
		case RUN_ARGS_MODE:

			// Allow the output parameter to be specified as part of
			// the artifact parameters but treat it especially.

			// In artifact run mode we allow the short flag -o to mean
			// --output
			if arg == "-o" {
				arg = "--output"
			}

			if utils.InString(choices, arg) {
				runmode_args = append(runmode_args, arg)
				state.Push(RUN_CLI_MODE)
				continue
			}

			// Immediately abort all parsing and show artifact help.
			if arg == "-h" || arg == "--help" {
				return append(prefix, []string{"artifacts", "collect",
					"--cli_help_mode", artifact_name}...), nil
			}

			if !strings.HasPrefix(arg, "--") {
				return nil, fmt.Errorf(
					"Run mode artifact parameters must start with `--`. Unexpected arg %v",
					arg)
			}

			current_artifact_arg = arg
			arg = strings.TrimPrefix(arg, "--")

			// Allow the flag to contain `=` or not
			if strings.Contains(arg, "=") {
				parts := strings.SplitN(arg, "=", 2)
				runmode_args = append(runmode_args,
					[]string{"--args", parts[0] + "=" + parts[1]}...)
				continue
			}

			// Need to see the arg parameter next
			state.Push(RUN_ARGS_VALUE_MODE)

		case RUN_ARGS_VALUE_MODE:
			flag := strings.TrimPrefix(current_artifact_arg, "--")
			runmode_args = append(runmode_args,
				[]string{"--args", flag + "=" + arg}...)
			state.Pop()

		case RUN_CLI_MODE:
			runmode_args = append(runmode_args, arg)
			state.Pop()

			// Something went horrible wrong!
		default:
			break
		}
	}

	// After parsing the commandline we need to know where state we
	// are left at so we can emit errors on short command lines.
	switch state.State() {
	case START_MODE:
		return prefix, nil

	case RUN_ARGS_MODE:
		res := append(prefix, runmode_args...)
		return res, nil

	case RUN_ARGS_VALUE_MODE:
		return nil, fmt.Errorf(
			"Expecting a value to follow flag `%v`",
			current_artifact_arg)

	case RUN_CLI_MODE:
		return nil, fmt.Errorf(
			"Expecting a value to follow `%v` flag", current_artifact_arg)

	case RUN_ARTIFACT_MODE:
		return nil, fmt.Errorf(
			"Expecting an artifact name to follow the --run flag")
	}

	return nil, fmt.Errorf("Failed to parse argv?")
}
