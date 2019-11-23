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
package parsers

import (
	"context"
	"errors"
	"io"
	"io/ioutil"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/oleparse"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _OLEVBAArgs struct {
	Filenames []string `vfilter:"required,field=file,doc=A list of filenames to open as OLE files."`
	Accessor  string   `vfilter:"optional,field=accessor,doc=The accessor to use."`
	MaxSize   int64    `vfilter:"optional,field=max_size,doc=Maximum size of file we load into memory."`
}

type _OLEVBAPlugin struct{}

func _OLEVBAPlugin_ParseFile(
	ctx context.Context,
	filename string,
	scope *vfilter.Scope,
	arg *_OLEVBAArgs) ([]*oleparse.VBAModule, error) {

	defer utils.CheckForPanic("Parsing VBA file.")

	accessor, err := glob.GetAccessor(arg.Accessor, ctx)
	if err != nil {
		return nil, err
	}

	fd, err := accessor.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	stat, err := fd.Stat()
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
		fd.Seek(0, io.SeekStart)
		data, err := ioutil.ReadAll(io.LimitReader(fd, constants.MAX_MEMORY))
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
		for _, f := range zfd.File {
			if oleparse.BINFILE_NAME.MatchString(f.Name) {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				data, err := ioutil.ReadAll(
					io.LimitReader(rc, constants.MAX_MEMORY))
				if err != nil {
					return nil, err
				}
				return oleparse.ParseBuffer(data)
			}
		}

	}

	return nil, errors.New("Not an OLE file.")
}

func (self _OLEVBAPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &_OLEVBAArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
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
				output_chan <- vql_subsystem.RowToDict(scope, macro_info).Set(
					"filename", filename)
			}
		}
	}()

	return output_chan
}

func (self _OLEVBAPlugin) Info(scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "olevba",
		Doc:     "Extracts VBA Macros from Office documents.",
		ArgType: type_map.AddType(scope, &_OLEVBAArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_OLEVBAPlugin{})
}
