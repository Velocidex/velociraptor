package parsers

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"howett.net/plist"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2021 Velocidex Innovations.
   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.
   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.
   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

type _PlistFunctionArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=file,doc=A list of files to parse."`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type PlistFunction struct{}

func (self PlistFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "plist",
		Doc:      "Parse plist file",
		ArgType:  type_map.AddType(scope, &_PlistFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func (self *PlistFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) (result vfilter.Any) {

	defer vql_subsystem.RegisterMonitor(ctx, "plist", args)()

	arg := &_PlistFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("plist: %s", err.Error())
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("plist: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.OpenWithOSPath(arg.Filename)
	if err != nil {
		scope.Log("plist: %v", err)
		return vfilter.Null{}
	}
	defer file.Close()

	var val interface{}
	dec := plist.NewDecoder(file)
	err = dec.Decode(&val)
	if err != nil {
		scope.Log("plist: %v", err)
		return vfilter.Null{}
	}

	// Force the results into dicts
	serialized, err := json.Marshal(val)
	if err != nil {
		scope.Log("plist: %v", err)
		return vfilter.Null{}
	}

	dicts, err := utils.ParseJsonToDicts(serialized)
	if err != nil {
		// We cant convert it to dicts, it might be something else,
		// just pass it as is (e.g. this happens with an array of
		// strings).
		return val
	}

	if len(dicts) == 1 {
		return dicts[0]
	}

	return dicts
}

type _PlistPluginArgs struct {
	Filenames []*accessors.OSPath `vfilter:"required,field=file,doc=A list of files to parse."`
	Accessor  string              `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type _PlistPlugin struct{}

func (self _PlistPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "plist",
		Doc:      "Parses a plist file.",
		ArgType:  type_map.AddType(scope, &_PlistPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func (self _PlistPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "plist", args)()

		arg := &_PlistPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("plist: %s", err.Error())
			return
		}

		for _, filename := range arg.Filenames {
			func() {
				defer utils.RecoverVQL(scope)

				accessor, err := accessors.GetAccessor(arg.Accessor, scope)
				if err != nil {
					scope.Log("plist: %v", err)
					return
				}

				file, err := accessor.OpenWithOSPath(filename)
				if err != nil {
					scope.Log("Unable to open file %s: %v",
						filename, err)
					return
				}

				defer file.Close()

				var val interface{}
				dec := plist.NewDecoder(file)
				err = dec.Decode(&val)
				if err != nil {
					scope.Log("plist: Unable to parse file %s: %v",
						filename, err)
				}

				select {
				case <-ctx.Done():
					return
				case output_chan <- val:
				}
			}()
		}
	}()

	return output_chan
}

func init() {
	vql_subsystem.RegisterFunction(&PlistFunction{})
	vql_subsystem.RegisterPlugin(&_PlistPlugin{})
}
