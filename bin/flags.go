package main

import (
	"os"

	"github.com/Velocidex/sflags"
	"github.com/Velocidex/sflags/gen/gkingpin"
	kingpin "github.com/alecthomas/kingpin/v2"
	proto "google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

func mergeFlagConfig(config_obj *config_proto.Config,
	default_config *config_proto.Config) error {

	enc_opts := json.NewEncOpts().WithOmitEmptyStructs()
	serialized, err := json.MarshalWithOptions(default_config, enc_opts)
	if err != nil {
		return err
	}

	default_config = &config_proto.Config{}
	err = json.Unmarshal(serialized, default_config)
	if err != nil {
		return err
	}

	proto.Merge(config_obj, default_config)
	return nil
}

func parseFlagsToDefaultConfig(app *kingpin.Application) (
	*config_proto.Config, error) {
	default_config := &config_proto.Config{
		Frontend: &config_proto.FrontendConfig{},
	}

	flags, err := sflags.ParseStruct(default_config, sflags.Prefix("config."))
	if err != nil {
		return nil, err
	}

	// Hide all the flags unless the verbose flag is on.
	if os.Getenv("DEBUG") == "" {
		for _, flag := range flags {
			flag.Hidden = true
		}
	}

	gkingpin.GenerateTo(flags, app)

	return default_config, nil
}
