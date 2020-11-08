package parsers

import (
	"context"
	"io"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	recyclebin "www.velocidex.com/golang/velociraptor/vql/parsers/recyclebin"
)

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.
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

type _RecycleBinPluginArgs struct {
	Filenames []string `vfilter:"required,field=filename,doc=Files to be parsed."`
	Accessor  string   `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type _RecycleBinPlugin struct{}


func (self _RecycleBinPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_recyclebin",
		Doc:     "Parses a $I file found in the $Recycle.Bin",
		ArgType: type_map.AddType(scope, &_RecycleBinPluginArgs{}),
	}
}

func (self _RecycleBinPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &_RecycleBinPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("parse_recyclebin: %s", err.Error())
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("parse_recyclebin: %s", err)
			return
		}

		for _, filename := range arg.Filenames {
			func() {
				defer utils.RecoverVQL(scope)
				
				err := vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
				if err != nil {
					scope.Log("parse_recyclebin: %s", err)
					return
				}
				
				accessor, err := glob.GetAccessor(arg.Accessor, scope)
				if err != nil {
					scope.Log("parse_recyclebin: %v", err)
					return
				}
				fd, err := accessor.Open(filename)
				if err != nil {
					scope.Log("parse_recyclebin: Unable to open file %s: %v",
						filename, err)
					return
				}
				defer fd.Close()

				reader, ok := fd.(io.ReaderAt)
				if !ok {
					scope.Log("parse_recyclebin: file is not seekable %s",
						filename)
					return
				}

				info, err := recyclebin.ParseRecycleBin(reader)
				if err != nil {
					scope.Log("parse_recyclebin: Unable to parse file %s: %v",
						filename, err)
					return
				}

				select {
				case <-ctx.Done():
					return

				case output_chan <- info:
				}
			}()
		}
	}()

	return output_chan
}


func init() {
	vql_subsystem.RegisterPlugin(&_RecycleBinPlugin{})
}
