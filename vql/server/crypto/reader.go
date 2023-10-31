package crypto

import (
	"context"
	"encoding/json"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/crypto/storage"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ReadCryptFilePluginArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=filename,doc=Path to the file to write"`
}

type ReadCryptFilePlugin struct{}

func (self ReadCryptFilePlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &ReadCryptFilePluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("read_crypto_file: %v", err)
			return
		}

		err = vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_READ)
		if err != nil {
			scope.Log("read_crypto_file: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("read_crypto_file: Must have access to server configuration", err)
			return
		}

		fd, err := storage.NewCryptoFileReader(ctx, config_obj, arg.Filename.String())
		if err != nil {
			scope.Log("write_crypto_file: %v", err)
			return
		}
		defer fd.Close()

		for packet := range fd.Parse(ctx) {
			if packet.VQLResponse == nil {
				continue
			}

			row := ordereddict.NewDict()
			err = json.Unmarshal([]byte(packet.VQLResponse.JSONLResponse), row)
			if err != nil {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self ReadCryptFilePlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "read_crypto_file",
		Doc:      "Read a previously stored encrypted local storage file.",
		ArgType:  type_map.AddType(scope, &ReadCryptFilePluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ReadCryptFilePlugin{})
}
