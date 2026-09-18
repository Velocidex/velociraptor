package frontend

import (
	"errors"
	"strings"

	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// Initializes the VQL environment for a minion.  This global change
// overrides plugins and functions which are not allowed to run on the
// minion wil redirects to the master. The minion can then process
// notebook VQL safely with some critical functionality directed to
// the master node.
func InitializeMinionVQL(config_obj *config_proto.Config) error {
	if config_obj.Frontend == nil ||
		config_obj.Frontend.PrivateKey == "" {
		return errors.New("Minion mode requires a frontend private key")
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Enabling minion VQL mode: some functions will be relayed to master!")

	completions, err := assets.LoadApiDescription()
	if err != nil {
		return err
	}

	scope := vql_subsystem.MakeScope()

	for _, completion := range completions {
		execution_context := completion.Metadata["execution"]
		if execution_context == "" {
			continue
		}

		if strings.EqualFold(completion.Type, "function") {
			fn, ok := scope.GetFunction(completion.Name)
			if !ok {
				continue
			}
			// Override the function with a related delegate.
			vql_subsystem.OverrideFunction(&RelayedFunction{
				Delegate:   fn,
				config_obj: config_obj,
			})

		} else if strings.EqualFold(completion.Type, "plugin") {
			pn, ok := scope.GetPlugin(completion.Name)
			if !ok {
				continue
			}
			// Override the function with a related delegate.
			vql_subsystem.OverridePlugin(&RelayedPlugin{
				Delegate:   pn,
				config_obj: config_obj,
			})
		}
	}
	return nil
}
