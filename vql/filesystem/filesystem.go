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
package filesystem

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/go-errors/errors"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/psutils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GlobPluginArgs struct {
	Globs               []string          `vfilter:"required,field=globs,doc=One or more glob patterns to apply to the filesystem."`
	Root                *accessors.OSPath `vfilter:"optional,field=root,doc=The root directory to glob from (default '')."`
	Accessor            string            `vfilter:"optional,field=accessor,doc=An accessor to use."`
	DoNotFollowSymlinks bool              `vfilter:"optional,field=nosymlink,doc=If set we do not follow symlinks."`
	RecursionCallback   string            `vfilter:"optional,field=recursion_callback,doc=A VQL function that determines if a directory should be recursed (e.g. \"x=>NOT x.Name =~ 'proc'\")."`
	OneFilesystem       bool              `vfilter:"optional,field=one_filesystem,doc=If set we do not follow links to other filesystems."`
}

type GlobPlugin struct{}

func (self GlobPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "glob", args)()

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			config_obj = &config_proto.Config{}
		}

		arg := &GlobPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("glob: %s", err.Error())
			return
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("glob: %v", err)
			return
		}

		// Expand glob braces over the entire expression - this allows
		// the alternatives to cover entire paths.
		globs := glob.ExpandBraces(arg.Globs)

		// Get the root of the glob. If not provided we use the
		// default root for the accessor.
		root := arg.Root
		accessor_root, err := accessor.ParsePath("")
		if err != nil {
			scope.Log("glob: %v", err)
			return
		}

		// Ensure the root has the require pathspec type by copying
		// the null manipulator.
		if root == nil {
			// Get the default top level path for this accessor.
			root = accessor_root
		} else {
			root.Manipulator = accessor_root.Manipulator
		}

		options := glob.GlobOptions{
			DoNotFollowSymlinks: arg.DoNotFollowSymlinks,
			OneFilesystem:       arg.OneFilesystem,
		}

		if arg.RecursionCallback != "" {
			// Compile the callback
			lambda, err := vfilter.ParseLambda(arg.RecursionCallback)
			if err != nil {
				scope.Log("glob: while parsing recursion_callback: %v", err)
				return
			}

			options.RecursionCallback = func(file_info accessors.FileInfo) bool {
				result := lambda.Reduce(ctx, scope, []vfilter.Any{file_info})
				return scope.Bool(result)
			}
		}

		globber := glob.NewGlobber().WithOptions(options)
		defer globber.Close()

		// If root is not specified we try to find a common
		// root from the globs.
		for _, item := range globs {

			if strings.HasPrefix(item, "{") {
				scope.Log("glob: Glob item appears to be a pathspec. This is deprecated, please use the root arg instead.")

				// This code attempts to emulate the old behavior for
				// backwards compatibility: The root is taken to be
				// the base pathspec and the glob is the Path
				// component.
				root, err = root.Parse(item)
				if err != nil {
					scope.Log("glob: %v", err)
					return
				}

				pathspec := root.PathSpec()
				item = pathspec.Path
				pathspec.Path = ""
				err = root.SetPathSpec(pathspec)
				if err != nil {
					scope.Log("glob: %v", err)
					return
				}
			}

			item_path, err := root.Parse(item)
			if err != nil {
				scope.Log("glob: %v", err)
				return
			}

			err = globber.Add(item_path)
			if err != nil {
				// Reject this expression but keep going - there may
				// be other globs.
				scope.Log("glob: Rejected glob expression %v: %v",
					item_path, err)
			}
		}

		file_chan := globber.ExpandWithContext(
			ctx, scope, config_obj, root, accessor)
		for f := range file_chan {
			select {
			case <-ctx.Done():
				return

			case output_chan <- f:
			}
		}
	}()

	return output_chan
}

func (self GlobPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "glob",
		Doc:      "Retrieve files based on a list of glob expressions",
		ArgType:  type_map.AddType(scope, &GlobPluginArgs{}),
		Version:  3,
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

type ReadFileArgs struct {
	Chunk     int                 `vfilter:"optional,field=chunk,doc=length of each chunk to read from the file."`
	MaxLength int                 `vfilter:"optional,field=max_length,doc=Max length of the file to read."`
	Filenames []*accessors.OSPath `vfilter:"required,field=filenames,doc=One or more files to open."`
	Accessor  string              `vfilter:"optional,field=accessor,doc=An accessor to use."`
}

type ReadFileResponse struct {
	Data     string
	Offset   int64
	Filename string
}

type ReadFilePlugin struct{}

func (self ReadFilePlugin) processFile(
	ctx context.Context,
	scope vfilter.Scope,
	arg *ReadFileArgs,
	accessor accessors.FileSystemAccessor,
	file *accessors.OSPath,
	output_chan chan vfilter.Row) {
	total_len := int64(0)

	fd, err := accessor.OpenWithOSPath(file)
	if err != nil {
		return
	}
	defer fd.Close()

	buf := make([]byte, arg.Chunk)
	for {
		n, err := io.ReadAtLeast(fd, buf, arg.Chunk)
		if err != nil &&
			!errors.Is(err, io.ErrUnexpectedEOF) &&
			!errors.Is(err, io.EOF) {
			scope.Log("read_file: %v", err)
			return
		}

		if n == 0 {
			return
		}
		response := &ReadFileResponse{
			Data:     string(buf[:n]),
			Offset:   total_len,
			Filename: file.String(),
		}

		select {
		case <-ctx.Done():
			return
		case output_chan <- response:
		}

		total_len += int64(n)

		if arg.MaxLength > 0 &&
			total_len > int64(arg.MaxLength) {
			break
		}
	}

}

func (self ReadFilePlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	defer vql_subsystem.RegisterMonitor(ctx, "read_file", args)()

	arg := &ReadFileArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
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
		defer vql_subsystem.RegisterMonitor(ctx, "read_file", args)()

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
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

func (self ReadFilePlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "read_file",
		Doc:      "Read files in chunks.",
		ArgType:  type_map.AddType(scope, &ReadFileArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

type ReadFileFunctionArgs struct {
	Length   int               `vfilter:"optional,field=length,doc=Max length of the file to read."`
	Offset   int64             `vfilter:"optional,field=offset,doc=Where to read from the file."`
	Filename *accessors.OSPath `vfilter:"required,field=filename,doc=One or more files to open."`
	Accessor string            `vfilter:"optional,field=accessor,doc=An accessor to use."`
}

type ReadFileFunction struct{}

func (self *ReadFileFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &ReadFileFunctionArgs{}

	defer vql_subsystem.RegisterMonitor(ctx, "read_file", args)()

	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("read_file: %s", err.Error())
		return ""
	}

	if arg.Length == 0 {
		arg.Length = 4 * 1024 * 1024
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("read_file: %v", err)
		return ""
	}

	buf := make([]byte, arg.Length)

	fd, err := accessor.OpenWithOSPath(arg.Filename)
	if err != nil {
		scope.Log("read_file: %v: %v", arg.Filename.String(), err)
		return ""
	}
	defer fd.Close()

	if arg.Offset > 0 {
		_, _ = fd.Seek(arg.Offset, os.SEEK_SET)
	}

	n, err := io.ReadAtLeast(fd, buf, len(buf))
	if err != nil &&
		!errors.Is(err, io.ErrUnexpectedEOF) &&
		!errors.Is(err, io.EOF) {
		scope.Log("read_file: %v", err)
		return ""
	}

	return string(buf[:n])
}

func (self ReadFileFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "read_file",
		Doc:      "Read a file into a string.",
		ArgType:  type_map.AddType(scope, &ReadFileFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

type StatArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=filename,doc=One or more files to open."`
	Accessor string            `vfilter:"optional,field=accessor,doc=An accessor to use."`
}

type StatPlugin struct{}

func (self *StatPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "stat", args)()

		arg := &StatArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("stat: %s", err.Error())
			return
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("stat: %s", err.Error())
			return
		}

		f, err := accessor.LstatWithOSPath(arg.Filename)
		if err == nil {
			select {
			case <-ctx.Done():
				return

			case output_chan <- f:
			}
		}
	}()

	return output_chan
}

func (self StatPlugin) Name() string {
	return "stat"
}

func (self StatPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "stat",
		Doc:      "Get file information. Unlike glob() this does not support wildcards.",
		ArgType:  type_map.AddType(scope, &StatArgs{}),
		Version:  2,
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

type StatFunction struct{}

func (self *StatFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "stat", args)()

	arg := &StatArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("stat: %s", err.Error())
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("stat: %s", err.Error())
		return vfilter.Null{}
	}

	f, err := accessor.LstatWithOSPath(arg.Filename)
	if err != nil {
		return vfilter.Null{}
	}

	return f
}

func (self StatFunction) Name() string {
	return "stat"
}

func (self StatFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "stat",
		Doc:      "Get file information. Unlike glob() this does not support wildcards.",
		ArgType:  type_map.AddType(scope, &StatArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&GlobPlugin{})
	vql_subsystem.RegisterPlugin(&ReadFilePlugin{})
	vql_subsystem.RegisterPlugin(
		vfilter.GenericListPlugin{
			PluginName: "filesystems",
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {
				var result []vfilter.Row
				partitions, err := psutils.PartitionsWithContext(ctx)
				if err == nil {
					for _, item := range partitions {
						result = append(result, item)
					}
				}
				return result
			},
		})
	vql_subsystem.RegisterPlugin(&StatPlugin{})
	vql_subsystem.RegisterFunction(&ReadFileFunction{})
	vql_subsystem.RegisterFunction(&StatFunction{})
}
