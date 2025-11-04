/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
package parsers

import (
	"context"
	"errors"
	"io"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/oleparse"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _OLEVBAArgs struct {
	Filenames []*accessors.OSPath `vfilter:"required,field=file,doc=A list of filenames to open as OLE files."`
	Accessor  string              `vfilter:"optional,field=accessor,doc=The accessor to use."`
	MaxSize   int64               `vfilter:"optional,field=max_size,doc=Maximum size of file we load into memory."`
}

type _OLEVBAPlugin struct{}

func _OLEVBAPlugin_ParseFile(
	ctx context.Context,
	filename *accessors.OSPath,
	scope vfilter.Scope,
	arg *_OLEVBAArgs) ([]*oleparse.VBAModule, error) {

	defer utils.RecoverVQL(scope)

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		return nil, err
	}

	fd, err := accessor.OpenWithOSPath(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	stat, err := accessor.LstatWithOSPath(filename)
	if err != nil {
		return nil, err
	}

	// Its a directory - not really an error just skip it.
	if stat.IsDir() {
		return nil, nil
	}

	signature := make([]byte, len(oleparse.OLE_SIGNATURE))
	_, err = io.ReadAtLeast(fd, signature, len(oleparse.OLE_SIGNATURE))
	if err != nil {
		return nil, err
	}

	if string(signature) == oleparse.OLE_SIGNATURE {
		// If the underlying file is not seekable we open it
		// again.
		_, err = fd.Seek(0, io.SeekStart)
		if err != nil {
			fd.Close()

			fd, err = accessor.OpenWithOSPath(filename)
			if err != nil {
				return nil, err
			}
			defer fd.Close()
		}

		max_memory := arg.MaxSize
		if max_memory == 0 {
			max_memory = constants.MAX_MEMORY
		}

		data, err := utils.ReadAllWithLimit(fd, int(max_memory))
		if err != nil {
			return nil, err
		}
		return oleparse.ParseBuffer(data)
	}

	// Maybe it is a zip file.
	reader, ok := fd.(io.ReaderAt)
	if !ok {
		return nil, errors.New("file is not seekable")
	}

	zfd, err := zip.NewReader(reader, stat.Size())
	if err == nil {
		results := []*oleparse.VBAModule{}
		for _, f := range zfd.File {
			if oleparse.BINFILE_NAME.MatchString(f.Name) {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				max_memory := constants.MAX_MEMORY
				if max_memory == 0 {
					max_memory = constants.MAX_MEMORY
				}

				data, err := utils.ReadAllWithLimit(rc, max_memory)
				if err != nil {
					return nil, err
				}
				modules, err := oleparse.ParseBuffer(data)
				if err == nil {
					results = append(results, modules...)
				}
			}
		}
		return results, nil
	}
	return nil, errors.New("Not an OLE file.")
}

func (self _OLEVBAPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "olevba", args)()

		arg := &_OLEVBAArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("olevba: %s", err.Error())
			return
		}

		for _, filename := range arg.Filenames {
			macros, err := _OLEVBAPlugin_ParseFile(ctx, filename, scope, arg)
			if err != nil {
				scope.Log("olevba: while parsing %v:  %s", filename, err)
				continue
			}

			for _, macro_info := range macros {
				select {
				case <-ctx.Done():
					return

				case output_chan <- vfilter.RowToDict(ctx, scope, macro_info).Set(
					"filename", filename):
				}
			}
		}
	}()

	return output_chan
}

func (self _OLEVBAPlugin) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "olevba",
		Doc:      "Extracts VBA Macros from Office documents.",
		ArgType:  type_map.AddType(scope, &_OLEVBAArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_OLEVBAPlugin{})
}
