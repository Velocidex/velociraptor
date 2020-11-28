package parsers

import (
	"context"
	"github.com/Velocidex/ordereddict"
	"howett.net/plist"
	"www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

/*
   Velociraptor - Hunting Evil
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
	Filename string `vfilter:"required,field=file,doc=A list of files to parse."`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type PlistFunction struct{}

func (self PlistFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "plist",
		Doc:     "Parse plist file",
		ArgType: type_map.AddType(scope, &_PlistFunctionArgs{}),
	}
}

func (self *PlistFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) (result vfilter.Any) {
	arg := &_PlistFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("plist: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("plist: %s", err)
		return
	}

	accessor, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("pslist: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.Open(arg.Filename)
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

	return val
}

type _PlistPluginArgs struct {
	Filenames []string `vfilter:"required,field=file,doc=A list of files to parse."`
	Accessor  string   `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type _PlistPlugin struct{}

func (self _PlistPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "plist",
		Doc:     "Parses a plist file.",
		ArgType: type_map.AddType(scope, &_PlistPluginArgs{}),
	}
}

func (self _PlistPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &_PlistPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("plist: %s", err.Error())
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("plist: %s", err)
			return
		}

		for _, filename := range arg.Filenames {
			func() {
				defer utils.RecoverVQL(scope)

				accessor, err := glob.GetAccessor(arg.Accessor, scope)
				if err != nil {
					scope.Log("plist: %v", err)
					return
				}

				file, err := accessor.Open(filename)
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
