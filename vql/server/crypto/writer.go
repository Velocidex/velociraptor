package crypto

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/crypto/storage"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type WriteCryptFilePluginArgs struct {
	Filename *accessors.OSPath   `vfilter:"required,field=filename,doc=Path to the file to write"`
	Query    vfilter.StoredQuery `vfilter:"required,field=query,doc=query to write into the file."`
	MaxWait  uint64              `vfilter:"optional,field=max_wait,doc=How often to flush the file (default 60 sec)."`
	MaxRows  uint64              `vfilter:"optional,field=max_rows,doc=How many rows to buffer before writing (default 1000)."`
	MaxSize  uint64              `vfilter:"optional,field=max_size,doc=When the file grows to this size, truncate it (default 1Gb)."`
}

type WriteCryptFilePlugin struct{}

func (self WriteCryptFilePlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "write_crypto_file", args)()

		arg := &WriteCryptFilePluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("write_crypto_file: %v", err)
			return
		}

		err = vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
		if err != nil {
			scope.Log("write_crypto_file: %v", err)
			return
		}

		// Make sure we are allowed to write there.
		err = file.CheckPrefix(arg.Filename)
		if err != nil {
			scope.Log("write_crypto_file: %v", err)
			return
		}

		config_obj_any, ok := scope.Resolve("config")
		if !ok {
			scope.Log("write_crypto_file: Must have access to client configuration", err)
			return
		}
		config_obj, ok := config_obj_any.(*config_proto.ClientConfig)
		if !ok {
			scope.Log("write_crypto_file: Must have access to client configuration", err)
			return
		}

		if arg.MaxRows == 0 {
			arg.MaxRows = 1000
		}

		if arg.MaxWait == 0 {
			arg.MaxWait = 60
		}

		if arg.MaxSize == 0 {
			arg.MaxSize = 1024 * 1024 * 1024 * 1024 // 1Gb
		}

		fd, err := storage.NewCryptoFileWriter(ctx, &config_proto.Config{
			Client: config_obj,
		}, arg.MaxSize, arg.Filename.String())
		if err != nil {
			scope.Log("write_crypto_file: %v", err)
			return
		}
		defer func() {
			err := fd.Close()

			if err != nil {
				scope.Log("write_crypto_file: %v", err)
			}
		}()

		// Flush the file periodically.
		go func() {
			for {
				select {
				case <-ctx.Done():
					return

				case <-time.After(time.Second * time.Duration(arg.MaxWait)):
					err := fd.Flush(!storage.KEEP_ON_ERROR)
					if err != nil {
						scope.Log("write_crypto_file: %v", err)
					}
				}
			}
		}()

		count := uint64(0)
		for row := range arg.Query.Eval(ctx, scope) {
			serialized_msg, err := json.Marshal(row)
			if err != nil {
				continue
			}
			serialized_msg = append(serialized_msg, '\n')
			fd.AddMessage(&crypto_proto.VeloMessage{
				VQLResponse: &actions_proto.VQLResponse{
					JSONLResponse: string(serialized_msg),
					Part:          count,
					TotalRows:     1,
				},
			})
			count++

			if count%arg.MaxRows == 0 {
				err := fd.Flush(!storage.KEEP_ON_ERROR)
				if err != nil {
					scope.Log("write_crypto_file: %v", err)
				}
			}

			// Echo the collected row out to our caller as well.
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self WriteCryptFilePlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "write_crypto_file",
		Doc:      "Write a query into an encrypted local storage file.",
		ArgType:  type_map.AddType(scope, &WriteCryptFilePluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&WriteCryptFilePlugin{})
}
