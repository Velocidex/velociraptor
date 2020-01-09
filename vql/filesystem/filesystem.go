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
package filesystem

import (
	"context"
	"io"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/disk"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type GlobPluginArgs struct {
	Globs    []string `vfilter:"required,field=globs,doc=One or more glob patterns to apply to the filesystem."`
	Accessor string   `vfilter:"optional,field=accessor,doc=An accessor to use."`
}

type GlobPlugin struct{}

func (self GlobPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	globber := make(glob.Globber)
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &GlobPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("glob: %s", err.Error())
			return
		}
		accessor, err := glob.GetAccessor(arg.Accessor, ctx)
		if err != nil {
			scope.Log("glob: %v", err)
			return
		}

		root := ""
		for _, item := range arg.Globs {
			item_root, item_path, _ := accessor.GetRoot(item)
			if root != "" && root != item_root {
				scope.Log("glob: %s: Must use the same root for "+
					"all globs. Skipping.", item)
				continue
			}
			root = item_root
			globber.Add(item_path, accessor.PathSplit)
		}

		file_chan := globber.ExpandWithContext(
			ctx, root, accessor)
		for {
			select {
			case <-ctx.Done():
				return

			case f, ok := <-file_chan:
				if !ok {
					return
				}
				output_chan <- f
			}
		}
	}()

	return output_chan
}

func (self GlobPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "glob",
		Doc:     "Retrieve files based on a list of glob expressions",
		RowType: type_map.AddType(scope, glob.NewVirtualDirectoryPath("", nil)),
		ArgType: type_map.AddType(scope, &GlobPluginArgs{}),
	}
}

type ReadFileArgs struct {
	Chunk     int      `vfilter:"optional,field=chunk,doc=length of each chunk to read from the file."`
	MaxLength int      `vfilter:"optional,field=max_length,doc=Max length of the file to read."`
	Filenames []string `vfilter:"required,field=filenames,doc=One or more files to open."`
	Accessor  string   `vfilter:"optional,field=accessor,doc=An accessor to use."`
}

type ReadFileResponse struct {
	Data     string
	Offset   int64
	Filename string
}

type ReadFilePlugin struct{}

func (self ReadFilePlugin) processFile(
	ctx context.Context,
	scope *vfilter.Scope,
	arg *ReadFileArgs,
	accessor glob.FileSystemAccessor,
	file string,
	output_chan chan vfilter.Row) {
	total_len := int64(0)

	fd, err := accessor.Open(file)
	if err != nil {
		return
	}
	defer fd.Close()

	buf := make([]byte, arg.Chunk)
	for {
		select {
		case <-ctx.Done():
			return

		default:
			n, err := io.ReadAtLeast(fd, buf, arg.Chunk)
			if err != nil &&
				errors.Cause(err) != io.ErrUnexpectedEOF &&
				errors.Cause(err) != io.EOF {
				scope.Log("read_file: %v", err)
				return
			}

			if n == 0 {
				return
			}
			response := &ReadFileResponse{
				Data:     string(buf[:n]),
				Offset:   total_len,
				Filename: file,
			}
			output_chan <- response
			total_len += int64(n)
		}
		if arg.MaxLength > 0 &&
			total_len > int64(arg.MaxLength) {
			break
		}
	}

}

func (self ReadFilePlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	arg := &ReadFileArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("%s: %s", self.Name(), err.Error())
		close(output_chan)
		return output_chan
	}

	if arg.Chunk == 0 {
		arg.Chunk = 4 * 1024 * 1024
	}

	go func() {
		defer close(output_chan)

		accessor, err := glob.GetAccessor(arg.Accessor, ctx)
		if err != nil {
			scope.Log("read_file: %v", err)
			return
		}

		for _, file := range arg.Filenames {
			self.processFile(
				ctx, scope, arg, accessor,
				file, output_chan)
		}
	}()

	return output_chan
}

func (self ReadFilePlugin) Name() string {
	return "read_file"
}

func (self ReadFilePlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "read_file",
		Doc:     "Read files in chunks.",
		RowType: type_map.AddType(scope, ReadFileResponse{}),
		ArgType: type_map.AddType(scope, &ReadFileArgs{}),
	}
}

type ReadFileFunctionArgs struct {
	Length   int    `vfilter:"optional,field=length,doc=Max length of the file to read."`
	Filename string `vfilter:"required,field=filename,doc=One or more files to open."`
	Accessor string `vfilter:"optional,field=accessor,doc=An accessor to use."`
}

type ReadFileFunction struct{}

func (self *ReadFileFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &ReadFileFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("read_file: %s", err.Error())
		return ""
	}

	if arg.Length == 0 {
		arg.Length = 4 * 1024 * 1024
	}

	accessor, err := glob.GetAccessor(arg.Accessor, ctx)
	if err != nil {
		scope.Log("read_file: %v", err)
		return ""
	}

	buf := make([]byte, arg.Length)

	fd, err := accessor.Open(arg.Filename)
	if err != nil {
		return ""
	}
	defer fd.Close()

	n, err := io.ReadAtLeast(fd, buf, len(buf))
	if err != nil &&
		errors.Cause(err) != io.ErrUnexpectedEOF &&
		errors.Cause(err) != io.EOF {
		scope.Log("read_file: %v", err)
		return ""
	}

	return string(buf[:n])
}

func (self ReadFileFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "read_file",
		Doc:     "Read a file into a string.",
		ArgType: type_map.AddType(scope, &ReadFileArgs{}),
	}
}

type StatArgs struct {
	Filename []string `vfilter:"required,field=filename,doc=One or more files to open."`
	Accessor string   `vfilter:"optional,field=accessor,doc=An accessor to use."`
}

type StatPlugin struct{}

func (self *StatPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &StatArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("%s: %s", "stat", err.Error())
			return
		}

		accessor, err := glob.GetAccessor(arg.Accessor, ctx)
		if err != nil {
			scope.Log("%s: %s", "stat", err.Error())
			return
		}
		for _, filename := range arg.Filename {
			f, err := accessor.Lstat(filename)
			if err == nil {
				output_chan <- f
			}
		}
	}()

	return output_chan
}

func (self StatPlugin) Name() string {
	return "stat"
}

func (self StatPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "stat",
		Doc:     "Get file information. Unlike glob() this does not support wildcards.",
		ArgType: type_map.AddType(scope, &StatArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&GlobPlugin{})
	vql_subsystem.RegisterPlugin(&ReadFilePlugin{})
	vql_subsystem.RegisterPlugin(
		vfilter.GenericListPlugin{
			PluginName: "filesystems",
			Function: func(
				scope *vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {
				var result []vfilter.Row
				partitions, err := disk.Partitions(true)
				if err == nil {
					for _, item := range partitions {
						result = append(result, item)
					}
				}
				return result
			},
			RowType: disk.PartitionStat{},
		})
	vql_subsystem.RegisterPlugin(&StatPlugin{})
	vql_subsystem.RegisterFunction(&ReadFileFunction{})
}
