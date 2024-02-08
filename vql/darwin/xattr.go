//go:build darwin
// +build darwin

package darwin

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/ivaxer/go-xattr"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type XAttrArgs struct {
	Filename  string `vfilter:"required,field=filename,doc=Filename to inspect."`
	Attribute string `vfilter:"required,field=attribute,doc=Attribute to collect."`
}

type XAttrPlugin struct{}

func (self XAttrPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.CheckForPanic(scope, "xattr")

		arg := &XAttrArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("xattr: %s", err.Error())
			return
		}

		var data []byte
		data, err = xattr.Get(arg.Filename, arg.Attribute)
		if err != nil {
			scope.Log("xattr: %s", err.Error())
			return
		}

		output_chan <- ordereddict.NewDict().
			Set("filename", arg.Filename).
			Set("attribute", arg.Attribute).
			Set("data", data)

	}()

	return output_chan
}

func (self XAttrPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "xattr",
		Doc:      "query a file for the specified extended attribute",
		ArgType:  type_map.AddType(scope, &XAttrArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&XAttrPlugin{})
}
