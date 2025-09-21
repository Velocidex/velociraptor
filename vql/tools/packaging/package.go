package packaging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/tools"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type CreatePackageArgs struct {
	Target      string            `vfilter:"optional,field=target,doc=The name of the target OS to repack (default VelociraptorLinux)"`
	Version     string            `vfilter:"optional,field=version,doc=Velociraptor Version to repack"`
	Release     string            `vfilter:"optional,field=release,doc=Rpm package release version (A)"`
	Server      bool              `vfilter:"optional,field=server,doc=Build a server rpm if true, otherwise we build a client rpm"`
	Exe         *accessors.OSPath `vfilter:"optional,field=exe,doc=Alternative a path to the executable to repack"`
	Accessor    string            `vfilter:"optional,field=accessor,doc=The accessor to use to read the file."`
	Config      string            `vfilter:"optional,field=config,doc=The config to be repacked in the form of a json or yaml string. If not provided we use the current config./"`
	ShowSpec    bool              `vfilter:"optional,field=show_spec,doc=If set we only show the spec that would have been used. You can use this to customize the input for package_spec"`
	DirName     string            `vfilter:"optional,field=directory_name,doc=Package files will be created inside this directory. If not specified we use a temporary directory"`
	ExtraArgs   []string          `vfilter:"optional,field=extra_args,doc=Additional command line args to be provided to the service"`
	PackageSpec *ordereddict.Dict `vfilter:"optional,field=package_spec,doc=A Package spec to use instead of the default, for ultimate customization"`
}

type CreatePackagePlugin struct {
	clientSpecFactory func() *PackageSpec
	serverSpecFactory func() *PackageSpec
	getArch           func(exe_bytes []byte) (string, error)
	builder           func(spec *PackageSpec) (Builder, error)

	name, description string
}

func (self CreatePackagePlugin) parseSpec(spec *ordereddict.Dict) (*PackageSpec, error) {
	serialized, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}

	result := &PackageSpec{}
	err = json.Unmarshal(serialized, result)
	if err != nil {
		return nil, err
	}

	if result.Files == nil {
		result.Files = ordereddict.NewDict()
	}

	for _, i := range result.Files.Items() {
		v_dict, ok := i.Value.(*ordereddict.Dict)
		if !ok {
			continue
		}

		fp := FileSpec{}
		mode, _ := v_dict.GetInt64("Mode")
		fp.Mode = uint(mode)
		fp.Owner, _ = v_dict.GetString("Owner")
		fp.Template, _ = v_dict.GetString("Template")

		result.Files.Update(i.Key, fp)
	}

	return result, nil
}

func (self CreatePackagePlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan types.Row {

	output_chan := make(chan types.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, self.name, args)()

		arg := &CreatePackageArgs{}
		err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
		if err != nil {
			scope.Log("ERROR:%v: %v", self.name, err)
			return
		}

		// To extract server config you need to be server admin.
		if arg.Server {
			err := vql_subsystem.CheckAccess(
				scope, acls.SERVER_ADMIN, acls.FILESYSTEM_WRITE)
			if err != nil {
				scope.Log("ERROR:%v: %v", self.name, err)
				return
			}
		}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("ERROR:%v: %v", self.name, err)
			return
		}

		if arg.Target == "" {
			arg.Target = "VelociraptorLinux"
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("ERROR:%v: Command can only run on the server", self.name)
			return
		}

		var target_config *config_proto.Config

		var spec *PackageSpec
		if arg.PackageSpec != nil {
			spec, err = self.parseSpec(arg.PackageSpec)
			if err != nil {
				scope.Log("ERROR:%v: %v", self.name, err)
				return
			}
			if spec.Server {
				target_config, err = validateServerConfig(config_obj)
				if err != nil {
					scope.Log("ERROR:%v: %v", self.name, err)
					return
				}

				// We force the binary to run as the velociraptor user
				target_config.Frontend.RunAsUser = spec.Expansion.ServerUser

			} else {
				target_config, err = validateClientConfig(config_obj, arg.Config)
				if err != nil {
					scope.Log("ERROR:%v: %v", self.name, err)
					return
				}
			}

		} else if arg.Server {
			spec = self.serverSpecFactory()
			target_config, err = validateServerConfig(config_obj)
			if err != nil {
				scope.Log("ERROR:%v: %v", self.name, err)
				return
			}

			// We force the binary to run as the velociraptor user
			target_config.Frontend.RunAsUser = spec.Expansion.ServerUser

		} else {
			spec = self.clientSpecFactory()
			target_config, err = validateClientConfig(config_obj, arg.Config)
			if err != nil {
				scope.Log("ERROR:%v: %v", self.name, err)
				return
			}
		}

		if arg.ShowSpec {
			pure_spec, _ := utils.ToPureDict(spec)
			output_chan <- ordereddict.NewDict().Set("Spec", pure_spec)
			return
		}

		// If arg.Version is not specified we select the latest version
		// available.
		exe_bytes, err := tools.ReadExeFile(ctx, config_obj, scope,
			arg.Exe, arg.Accessor, arg.Target, arg.Version)
		if err != nil {
			scope.Log("ERROR:%v: %v", self.name, err)
			return
		}

		arch, err := self.getArch(exe_bytes)
		if err != nil {
			scope.Log("ERROR:%v: %v", self.name, err)
			return
		}

		accessor, err := accessors.GetAccessor("auto", scope)
		if err != nil {
			scope.Log("ERROR:%v: %v", self.name, err)
			return
		}

		if arg.DirName == "" {
			arg.DirName = "."
		}

		abs_path, err := filepath.Abs(arg.DirName)
		if err != nil {
			scope.Log("ERROR:%v: %v", self.name, err)
			return
		}

		base_path, err := accessor.ParsePath(abs_path)
		if err != nil {
			scope.Log("ERROR:%v: %v", self.name, err)
			return
		}

		// Make sure we are allowed to write there.
		err = file.CheckPrefix(base_path)
		if err != nil {
			scope.Log("ERROR:%v: %v", self.name, err)
			return
		}

		for _, package_spec := range expandSpec(
			spec, target_config, arch, arg.Release, exe_bytes) {

			builder, err := self.builder(package_spec)
			if err != nil {
				scope.Log("ERROR:%v:Unable to create RPM: %w", self.name, err)
				return
			}

			filename := filepath.Join(arg.DirName, package_spec.OutputFilename())
			scope.Log("DEBUG:%v: writing file %v", self.name, filename)

			fd, err := os.OpenFile(filename,
				os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				scope.Log("ERROR:%v: %v", self.name, err)
				return
			}
			defer fd.Close()

			data, err := builder.Bytes(scope)
			if err != nil {
				scope.Log("ERROR:%v: %v", self.name, err)
				return
			}

			_, err = fd.Write(data)
			if err != nil {
				scope.Log("ERROR:%v: %v", self.name, err)
				return
			}

			output_chan <- ordereddict.NewDict().
				Set("Spec", spec).
				Set("OSPath", base_path.Append(package_spec.OutputFilename())).
				Set("Filename", filename)
		}
	}()

	return output_chan
}

func expandSpec(
	spec *PackageSpec,
	target_config *config_proto.Config,
	arch, release string,
	exe_bytes []byte) (res []*PackageSpec) {

	// This is a server
	if target_config.Frontend != nil {

		// There are minions.
		if len(target_config.ExtraFrontends) > 0 {
			res = append(res, spec.Copy().SetRuntimeParameters(
				target_config, arch, release, "master", 0, exe_bytes))

			for idx := range target_config.ExtraFrontends {
				res = append(res, spec.Copy().SetRuntimeParameters(
					target_config, arch, release, "minion", idx, exe_bytes))
			}

			// Just a regular server
		} else {
			res = append(res, spec.Copy().SetRuntimeParameters(
				target_config, arch, release, "server", 0, exe_bytes))
		}

		// It is a client
	} else {
		res = append(res, spec.Copy().SetRuntimeParameters(
			target_config, arch, release, "client", 0, exe_bytes))
	}

	return res
}

func validateServerConfig(config_obj *config_proto.Config) (*config_proto.Config, error) {
	res := proto.Clone(config_obj).(*config_proto.Config)
	if res.Frontend == nil || res.Client == nil {
		return nil, errors.New("Server Config requires a Frontend and Client sections.")
	}

	return res, nil
}

func validateClientConfig(
	// The current server config.
	config_obj *config_proto.Config,

	// A client config passed by the caller or "" for derving it from
	// the server config.
	config_yaml string) (stripped_config *config_proto.Config, err error) {

	// Derive client config from server config
	if config_yaml == "" {
		return config.GetClientConfig(config_obj), nil
	}

	// Load and validate the client config file.
	client_config, err := new(config.Loader).
		WithLiteralLoader([]byte(config_yaml)).
		WithRequiredClient().
		LoadAndValidate()
	if err != nil {
		return nil, fmt.Errorf("Invalid client config provided: %w", err)
	}

	// Strip any unrelated fields for the client config (in case the
	// user passed a server config accidentally).
	return config.GetClientConfig(client_config), nil
}

func (self CreatePackagePlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    self.name,
		Doc:     self.description,
		ArgType: type_map.AddType(scope, &CreatePackageArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.COLLECT_SERVER, acls.FILESYSTEM_WRITE, acls.SERVER_ADMIN).Build(),
	}
}
