package efi

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type EfiVariablesArgs struct {
	Namespace string `vfilter:"optional,field=namespace,doc=Variable namespace."`
	Name      string `vfilter:"optional,field=name,doc=Variable name"`
	Value     bool   `vfilter:"optional,field=value,doc=Read variable value"`
}

type EfiVariable struct {
	Namespace string
	Name      string
	Value     []byte
}

func runEfiVariables(
	ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) []vfilter.Row {
	var result []vfilter.Row

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Error("efivariables: %v", err)
		return result
	}

	arg := &EfiVariablesArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Error("efivariables: %v", err)
		return result
	}

	firmwareVariables, err := GetEfiVariables()
	if err != nil {
		scope.Error("efivariables: %v", err)
		return result
	}

	for _, item := range firmwareVariables {
		if (arg.Namespace == "" || arg.Namespace == item.Namespace) && (arg.Name == "" || arg.Name == item.Name) {
			if arg.Value {
				item.Value, err = GetEfiVariableValue(item.Namespace, item.Name)
				if err != nil {
					scope.Error("efivariables: %v %x", err, err)
					return result
				}
			}
			result = append(result, item)
		}
	}

	return result
}

func init() {
	vql_subsystem.RegisterPlugin(&vfilter.GenericListPlugin{
		PluginName: "efivariables",
		Doc:        "Enumerate efi variables.",
		Function:   runEfiVariables,
		ArgType:    &EfiVariablesArgs{},
		Metadata:   vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	})
}
